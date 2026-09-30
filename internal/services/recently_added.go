package services

import (
	"context"
	"fmt"
	"github.com/z354prance/overmynd/internal/integrations/tracearr"
	"github.com/z354prance/overmynd/internal/models"
	"sort"
	"time"
)

type RecentlyAddedItem struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Kind    string    `json:"kind"`
	Year    int       `json:"year,omitempty"`
	AddedAt time.Time `json:"added_at"`
	Stage   string    `json:"stage"`
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
		for _, kind := range []string{"movie", "episode"} {
			var page struct {
				Data []struct {
					ID        string     `json:"id"`
					ServerID  string     `json:"server_id"`
					RatingKey string     `json:"rating_key"`
					Title     string     `json:"title"`
					Year      int        `json:"year"`
					AddedAt   time.Time  `json:"added_at"`
					RemovedAt *time.Time `json:"removed_at"`
				} `json:"data"`
			}
			err := tracearr.NewClient().GetJSON(ctx, service.BaseURL, "recently-added?pageSize=5&media_type="+kind, key, &page)
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
				result.Items = append(result.Items, RecentlyAddedItem{ID: identity, Title: item.Title, Kind: kind, Year: item.Year, AddedAt: item.AddedAt, Stage: "available"})
			}
		}
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].AddedAt.Equal(result.Items[j].AddedAt) {
			return result.Items[i].ID < result.Items[j].ID
		}
		return result.Items[i].AddedAt.After(result.Items[j].AddedAt)
	})
	if len(result.Items) > 5 {
		result.Items = result.Items[:5]
	}
	return result, nil
}
