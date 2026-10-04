package seerr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
)

// UserJSON always acts as the explicitly configured user, never as the API-key
// owner. Seerr applies this user's request permissions, quotas and auto-approval.
func UserJSON(ctx context.Context, baseURL, apiKey string, userID int64, method, path string, input, output any) error {
	if userID <= 0 {
		return fmt.Errorf("a Seerr request user must be configured")
	}
	base, err := integrations.NormalizeBaseURL(baseURL)
	if err != nil {
		return fmt.Errorf("invalid Seerr configuration")
	}
	var body []byte
	if input != nil {
		body, err = json.Marshal(input)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, base+"/api/v1/"+path, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to create Seerr request")
	}
	req.Header.Set("X-Api-Key", apiKey)
	req.Header.Set("X-API-User", strconv.FormatInt(userID, 10))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("unable to reach Seerr; try again later")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Never return upstream bodies, URLs or credentials to public callers.
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

func SanitizeResults(results []SearchResult) []SearchResult {
	clean := make([]SearchResult, 0)
	for _, item := range results {
		if item.ID <= 0 || (item.MediaType != "movie" && item.MediaType != "tv") {
			continue
		}
		if !strings.HasPrefix(item.PosterPath, "/") || strings.Contains(item.PosterPath, "..") {
			item.PosterPath = ""
		}
		clean = append(clean, item)
		if len(clean) == 20 {
			break
		}
	}
	return clean
}
