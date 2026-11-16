// Package tracearr reads Tracearr's public API: who is playing, and what was added
// to the library. Ported from Overmynd so the public page shows the same things.
package tracearr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"), Token: strings.TrimSpace(token),
		HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/v2/public/"+strings.TrimLeft(path, "/"), nil)
	if err != nil {
		return fmt.Errorf("Tracearr request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Undermynd")
	req.Header.Set("Authorization", "Bearer "+c.Token)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("Tracearr request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return fmt.Errorf("Tracearr response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("Tracearr returned HTTP %d", resp.StatusCode)
	}
	if out == nil || len(body) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("Tracearr JSON: %w", err)
	}
	return nil
}

// Check proves the address and key work.
func (c *Client) Check(ctx context.Context) error {
	if c.Token == "" {
		return fmt.Errorf("Tracearr Public API key is required")
	}
	return c.get(ctx, "streams?summary=true", nil)
}

// Session is one current stream, in the shape the dashboard reads.
type Session struct {
	ID         string `json:"id"`
	ServerID   string `json:"server_id,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	ServerType string `json:"server_type,omitempty"`
	Username   string `json:"username,omitempty"`

	MediaTitle    string `json:"media_title"`
	MediaType     string `json:"media_type,omitempty"`
	ShowTitle     string `json:"show_title,omitempty"`
	SeasonNumber  int    `json:"season_number,omitempty"`
	EpisodeNumber int    `json:"episode_number,omitempty"`
	Year          int    `json:"year,omitempty"`

	ArtistName  string `json:"artist_name,omitempty"`
	AlbumName   string `json:"album_name,omitempty"`
	TrackNumber int    `json:"track_number,omitempty"`

	DurationMs int64      `json:"duration_ms,omitempty"`
	ProgressMs int64      `json:"progress_ms,omitempty"`
	State      string     `json:"state,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`

	IsTranscode   bool   `json:"is_transcode"`
	VideoDecision string `json:"video_decision,omitempty"`
	AudioDecision string `json:"audio_decision,omitempty"`
	Bitrate       int64  `json:"bitrate,omitempty"`

	Device   string `json:"device,omitempty"`
	Player   string `json:"player,omitempty"`
	Product  string `json:"product,omitempty"`
	Platform string `json:"platform,omitempty"`

	RatingKey string   `json:"rating_key,omitempty"`
	LibraryID string   `json:"library_id,omitempty"`
	Genres    []string `json:"genres,omitempty"`
	PosterURL string   `json:"poster_url,omitempty"`
}

type stream struct {
	Session
	ArtistName  *string         `json:"artist_name"`
	AlbumName   *string         `json:"album_name"`
	TrackNumber *int            `json:"track_number"`
	StartedAt   string          `json:"started_at"`
	TMDBID      json.RawMessage `json:"tmdb_id"`
}

// Streams lists what is playing now.
func (c *Client) Streams(ctx context.Context) ([]Session, error) {
	var r struct {
		Data []stream `json:"data"`
	}
	if err := c.get(ctx, "streams", &r); err != nil {
		return nil, err
	}
	out := make([]Session, 0, len(r.Data))
	for _, x := range r.Data {
		s := x.Session
		if x.ArtistName != nil {
			s.ArtistName = *x.ArtistName
		}
		if x.AlbumName != nil {
			s.AlbumName = *x.AlbumName
		}
		if x.TrackNumber != nil {
			s.TrackNumber = *x.TrackNumber
		}
		if t, err := time.Parse(time.RFC3339, x.StartedAt); err == nil {
			s.StartedAt = &t
		}
		out = append(out, s)
	}
	return out, nil
}

// Item is one recently added title for the dashboard; a show's new episodes are one item.
type Item struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	ShowTitle string    `json:"show_title,omitempty"`
	ShowID    string    `json:"show_id,omitempty"`
	Episodes  []string  `json:"episodes,omitempty"`
	Kind      string    `json:"kind"`
	Year      int       `json:"year,omitempty"`
	AddedAt   time.Time `json:"added_at"`
	Stage     string    `json:"stage"`

	PosterURL string `json:"poster_url,omitempty"`

	mediaID string
}

type RecentResult struct {
	Items      []Item   `json:"items"`
	Errors     []string `json:"errors"`
	Configured bool     `json:"configured"`
}

// RecentlyAdded reads confirmed library additions (not disappearing queue jobs):
// the newest six movies or shows, with a show's new episodes grouped.
func (c *Client) RecentlyAdded(ctx context.Context) RecentResult {
	result := RecentResult{Items: []Item{}, Errors: []string{}, Configured: true}
	seen := map[string]bool{}
	feedCtx, cancelFeed := context.WithTimeout(ctx, 15*time.Second)
	defer cancelFeed()
kinds:
	for _, kind := range []string{"movie", "episode"} {
		cursor := ""
		cursors := map[string]bool{}
		distinct := map[string]bool{}
		for page := 0; page < 20; page++ {
			var p struct {
				Data []struct {
					ID         string     `json:"id"`
					MediaID    string     `json:"media_id"`
					ShowKey    string     `json:"grandparent_rating_key"`
					ServerID   string     `json:"server_id"`
					ServerType string     `json:"server_type"`
					RatingKey  string     `json:"rating_key"`
					Title      string     `json:"title"`
					Year       int        `json:"year"`
					AddedAt    time.Time  `json:"added_at"`
					RemovedAt  *time.Time `json:"removed_at"`
				} `json:"data"`
				Meta struct {
					NextCursor string `json:"nextCursor"`
				} `json:"meta"`
			}
			path := "recently-added?pageSize=100&media_type=" + kind
			if cursor != "" {
				path += "&cursor=" + url.QueryEscape(cursor)
			}
			if err := c.get(feedCtx, path, &p); err != nil {
				result.Errors = append(result.Errors, "Recently added is unavailable from Tracearr. Check that its version supports the recently-added public API and library sync is enabled.")
				break kinds
			}
			for _, it := range p.Data {
				if it.ID == "" || it.AddedAt.IsZero() || it.RemovedAt != nil {
					continue
				}
				identity := it.ServerID + ":" + it.RatingKey
				if it.RatingKey == "" {
					identity = "0:" + it.ID
				}
				if seen[identity] {
					continue
				}
				seen[identity] = true
				group := identity
				showID := ""
				if kind == "episode" && it.ShowKey != "" {
					group = it.ServerID + ":" + it.ShowKey
					showID = "0:" + group
				}
				distinct[group] = true
				result.Items = append(result.Items, Item{PosterURL: recentPosterPath(it.ServerID, it.ServerType, it.RatingKey, it.ShowKey, kind), ShowID: showID, mediaID: it.MediaID, ID: identity, Title: it.Title,
					Kind: kind, Year: it.Year, AddedAt: it.AddedAt, Stage: "available"})
			}
			if len(distinct) >= 6 || p.Meta.NextCursor == "" {
				break
			}
			if page == 19 || cursors[p.Meta.NextCursor] {
				result.Errors = append(result.Errors, "Recently added reached its history limit; some older shows may be missing.")
				break
			}
			cursor = p.Meta.NextCursor
			cursors[cursor] = true
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].AddedAt.Equal(result.Items[j].AddedAt) {
			return result.Items[i].ID < result.Items[j].ID
		}
		return result.Items[i].AddedAt.After(result.Items[j].AddedAt)
	})
	result.Items = groupShows(result.Items)
	if len(result.Items) > 6 {
		result.Items = result.Items[:6]
	}
	// Resolve episode -> show from canonical media IDs, never guess from episode titles.
	type detail struct {
		Title  string `json:"title"`
		ShowID string `json:"show_media_id"`
	}
	cache := map[string]detail{}
	fetch := func(id string) (detail, error) {
		if v, ok := cache[id]; ok {
			return v, nil
		}
		var v detail
		err := c.get(ctx, "media/"+url.PathEscape(id), &v)
		if err == nil {
			cache[id] = v
		}
		return v, err
	}
	for i := range result.Items {
		it := &result.Items[i]
		if (it.Kind != "episode" && it.Kind != "series") || it.mediaID == "" {
			continue
		}
		ep, err := fetch(it.mediaID)
		if err != nil || ep.ShowID == "" {
			continue
		}
		if show, err := fetch(ep.ShowID); err == nil {
			it.ShowTitle = show.Title
			if it.Kind == "series" {
				it.Title = show.Title
			}
		}
	}
	return result
}

func recentPosterPath(server, serverType, ratingKey, showKey, kind string) string {
	if kind == "episode" && showKey != "" {
		ratingKey = showKey
	}
	if server == "" || ratingKey == "" {
		return ""
	}
	switch serverType {
	case "emby", "jellyfin":
		return "/api/v1/images/proxy?" + url.Values{"server": {server}, "url": {"/Items/" + url.PathEscape(ratingKey) + "/Images/Primary"}}.Encode()
	}
	return "" // Plex needs a timestamped thumb path this feed lacks
}

// Input is newest first. Group by server and show identity, never by title.
func groupShows(items []Item) []Item {
	out := []Item{}
	groups := map[string]int{}
	for _, it := range items {
		if it.Kind != "episode" || it.ShowID == "" {
			out = append(out, it)
			continue
		}
		if i, ok := groups[it.ShowID]; ok {
			out[i].Episodes = append(out[i].Episodes, it.Title)
			continue
		}
		groups[it.ShowID] = len(out)
		it.Episodes = []string{it.Title}
		it.Kind = "series"
		it.ID = "recent-show:" + it.ShowID
		it.Title = "Recently added episodes"
		out = append(out, it)
	}
	return out
}

var _ = strconv.Itoa
