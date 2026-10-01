package services

import (
	"context"
	"fmt"
	"github.com/z354prance/overmynd/internal/integrations/tracearr"
	"github.com/z354prance/overmynd/internal/models"
	"net/url"
	"sort"
	"time"
)

type RecentlyAddedItem struct {
	ShowTitle string   `json:"show_title,omitempty"`
	ShowID    string   `json:"show_id,omitempty"`
	Episodes  []string `json:"episodes,omitempty"`
	mediaID   string
	serviceID int64
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Kind      string    `json:"kind"`
	Year      int       `json:"year,omitempty"`
	AddedAt   time.Time `json:"added_at"`
	Stage     string    `json:"stage"`
}
type RecentlyAddedResult struct {
	Items      []RecentlyAddedItem `json:"items"`
	Errors     []string            `json:"errors"`
	Configured bool                `json:"configured"`
}

// RecentlyAdded uses confirmed library additions, not disappearing queue jobs.
func (m *Manager) RecentlyAdded(ctx context.Context) (RecentlyAddedResult, error) {
	result := RecentlyAddedResult{Items: []RecentlyAddedItem{}, Errors: []string{}}
	services, err := m.List()
	if err != nil {
		return result, err
	}
	seen := map[string]bool{}
	for _, service := range services {
		if !service.Enabled || service.Type != models.ServiceTracearr {
			continue
		}
		result.Configured = true
		key, err := m.Credential(service.ID)
		if err != nil {
			result.Errors = append(result.Errors, "Unable to read Tracearr configuration")
			continue
		}
		feedCtx, cancelFeed := context.WithTimeout(ctx, 15*time.Second)
		errorCount := len(result.Errors)
		for _, kind := range []string{"movie", "episode"} {
			cursor := ""
			cursors := map[string]bool{}
			distinct := map[string]bool{}
			for pageNumber := 0; pageNumber < 20; pageNumber++ {
				var page struct {
					Data []struct {
						ID        string     `json:"id"`
						MediaID   string     `json:"media_id"`
						ShowKey   string     `json:"grandparent_rating_key"`
						ServerID  string     `json:"server_id"`
						RatingKey string     `json:"rating_key"`
						Title     string     `json:"title"`
						Year      int        `json:"year"`
						AddedAt   time.Time  `json:"added_at"`
						RemovedAt *time.Time `json:"removed_at"`
					} `json:"data"`
					Meta struct {
						NextCursor string `json:"nextCursor"`
					} `json:"meta"`
				}
				path := "recently-added?pageSize=100&media_type=" + kind
				if cursor != "" {
					path += "&cursor=" + url.QueryEscape(cursor)
				}
				err := tracearr.NewClient().GetJSON(feedCtx, service.BaseURL, path, key, &page)
				if err != nil {
					result.Errors = append(result.Errors, "Recently added is unavailable from Tracearr. Check that its version supports the recently-added public API and library sync is enabled.")
					break
				}
				for _, item := range page.Data {
					if item.ID == "" || item.AddedAt.IsZero() || item.RemovedAt != nil {
						continue
					}
					identity := item.ServerID + ":" + item.RatingKey
					if item.RatingKey == "" {
						identity = fmt.Sprintf("%d:%s", service.ID, item.ID)
					}
					if seen[identity] {
						continue
					}
					seen[identity] = true
					group := identity
					if kind == "episode" && item.ShowKey != "" {
						group = item.ServerID + ":" + item.ShowKey
					}
					distinct[group] = true
					result.Items = append(result.Items, RecentlyAddedItem{ShowID: func() string {
						if kind == "episode" && item.ShowKey != "" {
							return fmt.Sprintf("%d:%s:%s", service.ID, item.ServerID, item.ShowKey)
						}
						return ""
					}(), mediaID: item.MediaID, serviceID: service.ID, ID: identity, Title: item.Title, Kind: kind, Year: item.Year, AddedAt: item.AddedAt, Stage: "available"})
				}
				if len(distinct) >= 6 || page.Meta.NextCursor == "" {
					break
				}
				if pageNumber == 19 || cursors[page.Meta.NextCursor] {
					result.Errors = append(result.Errors, "Recently added reached its history limit; some older shows may be missing.")
					break
				}
				cursor = page.Meta.NextCursor
				cursors[cursor] = true
			}
			if len(result.Errors) > errorCount {
				break
			}
		}
		cancelFeed()
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].AddedAt.Equal(result.Items[j].AddedAt) {
			return result.Items[i].ID < result.Items[j].ID
		}
		return result.Items[i].AddedAt.After(result.Items[j].AddedAt)
	})
	result.Items = groupRecentShows(result.Items)
	if len(result.Items) > 6 {
		result.Items = result.Items[:6]
	}
	// Resolve episode -> show from canonical media IDs, never guess from episode titles.
	enrichCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	type detail struct {
		Title  string `json:"title"`
		ShowID string `json:"show_media_id"`
	}
	cache := map[string]detail{}
	for i := range result.Items {
		item := &result.Items[i]
		if (item.Kind != "episode" && item.Kind != "series") || item.mediaID == "" {
			continue
		}
		service, err := m.Get(item.serviceID)
		if err != nil {
			continue
		}
		key, err := m.Credential(item.serviceID)
		if err != nil {
			continue
		}
		fetch := func(id string) (detail, error) {
			cacheKey := fmt.Sprintf("%d:%s", item.serviceID, id)
			if value, ok := cache[cacheKey]; ok {
				return value, nil
			}
			var value detail
			err := tracearr.NewClient().GetJSON(enrichCtx, service.BaseURL, "media/"+url.PathEscape(id), key, &value)
			if err == nil {
				cache[cacheKey] = value
			}
			return value, err
		}
		episode, err := fetch(item.mediaID)
		if err != nil || episode.ShowID == "" {
			continue
		}
		show, err := fetch(episode.ShowID)
		if err == nil {
			item.ShowTitle = show.Title
			if item.Kind == "series" {
				item.Title = show.Title
			}
		}
	}
	return result, nil
}

// Input is newest first. Group by service/server/show identity, never by title.
func groupRecentShows(items []RecentlyAddedItem) []RecentlyAddedItem {
	result := []RecentlyAddedItem{}
	groups := map[string]int{}
	for _, item := range items {
		if item.Kind != "episode" || item.ShowID == "" {
			result = append(result, item)
			continue
		}
		if index, ok := groups[item.ShowID]; ok {
			result[index].Episodes = append(result[index].Episodes, item.Title)
			continue
		}
		groups[item.ShowID] = len(result)
		item.Episodes = []string{item.Title}
		item.Kind = "series"
		item.ID = "recent-show:" + item.ShowID
		item.Title = "Recently added episodes"
		result = append(result, item)
	}
	return result
}
