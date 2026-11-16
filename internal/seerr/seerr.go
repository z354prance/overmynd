// Package seerr talks to Seerr (Overseerr/Jellyseerr) for the public request box.
// Ported from Overmynd: every public call acts as one configured Seerr user, so
// Seerr's own permissions, quotas and auto-approval apply.
package seerr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Key     string
	UserID  int64 // 0 acts as the API key's owner (admin calls only)
}

func New(baseURL, key string, userID int64) *Client {
	return &Client{BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), Key: strings.TrimSpace(key), UserID: userID}
}

var noRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

// JSON performs one call. Upstream bodies, URLs and keys never reach the caller.
func (c *Client) JSON(ctx context.Context, method, path string, input, output any) error {
	if !strings.HasPrefix(c.BaseURL, "http://") && !strings.HasPrefix(c.BaseURL, "https://") {
		return fmt.Errorf("invalid Seerr configuration")
	}
	var body []byte
	if input != nil {
		var err error
		if body, err = json.Marshal(input); err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+"/api/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to create Seerr request")
	}
	req.Header.Set("X-Api-Key", c.Key)
	if c.UserID > 0 {
		req.Header.Set("X-API-User", strconv.FormatInt(c.UserID, 10))
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 15 * time.Second, CheckRedirect: noRedirect}).Do(req)
	if err != nil {
		return fmt.Errorf("unable to reach Seerr; try again later")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case 401, 403:
			return fmt.Errorf("Seerr denied access for the configured request user")
		case 409:
			return fmt.Errorf("this title has already been requested")
		case 429:
			return fmt.Errorf("Seerr request limit reached; try again later")
		default:
			return fmt.Errorf("Seerr could not complete the request (HTTP %d); check the user's permissions, quota, or existing requests", resp.StatusCode)
		}
	}
	if output == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(output); err != nil {
		return fmt.Errorf("invalid response from Seerr")
	}
	return nil
}

// CheckUser proves the address, the key, and that the key can act as the user.
func (c *Client) CheckUser(ctx context.Context) error {
	if c.UserID <= 0 {
		return fmt.Errorf("enter the Seerr user ID that requests are made as")
	}
	var u struct {
		ID int64 `json:"id"`
	}
	if err := c.JSON(ctx, "GET", "auth/me", nil, &u); err != nil {
		return err
	}
	if u.ID != c.UserID {
		return fmt.Errorf("Seerr did not authenticate as the selected user")
	}
	return nil
}

type SearchResult struct {
	ID           int64  `json:"id"`
	MediaType    string `json:"mediaType"`
	Title        string `json:"title,omitempty"`
	Name         string `json:"name,omitempty"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	FirstAirDate string `json:"firstAirDate,omitempty"`
	Overview     string `json:"overview,omitempty"`
	PosterPath   string `json:"posterPath,omitempty"`
	MediaInfo    *struct {
		Status int `json:"status"`
	} `json:"mediaInfo,omitempty"`
}

func (c *Client) Search(ctx context.Context, query string) ([]SearchResult, error) {
	var r struct {
		Results []SearchResult `json:"results"`
	}
	q := strings.ReplaceAll(url.Values{"query": {query}, "page": {"1"}}.Encode(), "+", "%20")
	if err := c.JSON(ctx, "GET", "search?"+q, nil, &r); err != nil {
		return nil, err
	}
	clean := make([]SearchResult, 0)
	for _, it := range r.Results {
		if it.ID <= 0 || (it.MediaType != "movie" && it.MediaType != "tv") {
			continue
		}
		if !strings.HasPrefix(it.PosterPath, "/") || strings.Contains(it.PosterPath, "..") {
			it.PosterPath = ""
		}
		clean = append(clean, it)
		if len(clean) == 20 {
			break
		}
	}
	return clean, nil
}

// Discover lists Seerr's suggestions: "trending", "movies" (popular) or "tv" (popular).
func (c *Client) Discover(ctx context.Context, kind string) ([]SearchResult, error) {
	path, mediaType := "", ""
	switch kind {
	case "trending":
		path = "discover/trending"
	case "movies":
		path, mediaType = "discover/movies", "movie"
	case "tv":
		path, mediaType = "discover/tv", "tv"
	default:
		return nil, fmt.Errorf("unknown list")
	}
	var r struct {
		Results []SearchResult `json:"results"`
	}
	if err := c.JSON(ctx, "GET", path+"?page=1", nil, &r); err != nil {
		return nil, err
	}
	out := make([]SearchResult, 0, 20)
	for _, it := range r.Results {
		if it.MediaType == "" {
			it.MediaType = mediaType
		}
		if it.ID <= 0 || (it.MediaType != "movie" && it.MediaType != "tv") {
			continue
		}
		if !strings.HasPrefix(it.PosterPath, "/") || strings.Contains(it.PosterPath, "..") {
			it.PosterPath = ""
		}
		out = append(out, it)
		if len(out) == 20 {
			break
		}
	}
	return out, nil
}

// Request sends one request with no caller-controlled user, approval, quota, server or profile.
func (c *Client) Request(ctx context.Context, mediaID int64, mediaType string) (int64, error) {
	payload := map[string]any{"mediaId": mediaID, "mediaType": mediaType}
	if mediaType == "tv" {
		payload["seasons"] = "all"
	}
	var r struct {
		ID int64 `json:"id"`
	}
	if err := c.JSON(ctx, "POST", "request", payload, &r); err != nil {
		return 0, err
	}
	if r.ID <= 0 {
		return 0, fmt.Errorf("Seerr did not confirm a request; check Seerr before retrying")
	}
	return r.ID, nil
}

// Pending is one request awaiting approval, in the shape the public page reads.
type Pending struct {
	ID              string    `json:"id"`
	Source          string    `json:"source"`
	SourceServiceID int64     `json:"source_service_id"`
	SourceID        int64     `json:"source_id"`
	MediaType       string    `json:"media_type"`
	Title           string    `json:"title,omitempty"`
	TMDBID          int64     `json:"tmdb_id,omitempty"`
	Status          string    `json:"status"`
	RequestedBy     string    `json:"requested_by,omitempty"`
	Seasons         []int     `json:"seasons,omitempty"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
}

func (c *Client) PendingRequests(ctx context.Context) ([]Pending, error) {
	var r struct {
		Results []struct {
			ID        int64     `json:"id"`
			Status    int       `json:"status"`
			CreatedAt time.Time `json:"createdAt"`
			Media     struct {
				MediaType string `json:"mediaType"`
				TMDBID    int64  `json:"tmdbId"`
			} `json:"media"`
			RequestedBy struct {
				DisplayName string `json:"displayName"`
				Username    string `json:"username"`
			} `json:"requestedBy"`
			Seasons []struct {
				SeasonNumber int `json:"seasonNumber"`
			} `json:"seasons"`
		} `json:"results"`
	}
	if err := c.JSON(ctx, "GET", "request?take=100&skip=0&sort=added&filter=pending", nil, &r); err != nil {
		return nil, err
	}
	out := []Pending{}
	for _, x := range r.Results {
		if x.Status != 1 {
			continue
		}
		p := Pending{ID: "seerr:request:" + strconv.FormatInt(x.ID, 10), Source: "seerr", SourceID: x.ID, MediaType: strings.ToLower(x.Media.MediaType),
			TMDBID: x.Media.TMDBID, Status: "pending", CreatedAt: x.CreatedAt}
		if p.RequestedBy = strings.TrimSpace(x.RequestedBy.DisplayName); p.RequestedBy == "" {
			p.RequestedBy = strings.TrimSpace(x.RequestedBy.Username)
		}
		for _, s := range x.Seasons {
			p.Seasons = append(p.Seasons, s.SeasonNumber)
		}
		if p.TMDBID > 0 && (p.MediaType == "movie" || p.MediaType == "tv") {
			var d struct{ Title, Name string }
			if c.JSON(ctx, "GET", p.MediaType+"/"+strconv.FormatInt(p.TMDBID, 10), nil, &d) == nil {
				p.Title = strings.TrimSpace(d.Title + d.Name)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

// Decide approves or declines a request, but only while it still awaits approval.
func (c *Client) Decide(ctx context.Context, requestID int64, action string) error {
	if requestID <= 0 || (action != "approve" && action != "decline") {
		return fmt.Errorf("invalid request decision")
	}
	var cur struct {
		Status int `json:"status"`
	}
	p := "request/" + strconv.FormatInt(requestID, 10)
	if err := c.JSON(ctx, "GET", p, nil, &cur); err != nil {
		return fmt.Errorf("unable to check the request in Seerr")
	}
	if cur.Status != 1 {
		return fmt.Errorf("the request is no longer awaiting approval; refresh the list")
	}
	return c.JSON(ctx, "POST", p+"/"+action, nil, nil)
}
