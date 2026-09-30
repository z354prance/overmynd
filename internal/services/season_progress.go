package services

import (
	"context"
	"fmt"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
	"sort"
)

type SeasonProgress struct {
	ShowID    string `json:"show_id"`
	ShowTitle string `json:"show_title"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Season    int    `json:"season_number"`
	Imported  int    `json:"imported"`
	Total     int    `json:"total"`
}
type seasonEpisode struct {
	ID        int64 `json:"id"`
	Season    int   `json:"seasonNumber"`
	Monitored bool  `json:"monitored"`
	HasFile   bool  `json:"hasFile"`
}

func countSeasonProgress(episodes []seasonEpisode, queued map[int64]bool) map[int]SeasonProgress {
	active := map[int]bool{}
	for _, ep := range episodes {
		if queued[ep.ID] {
			active[ep.Season] = true
		}
	}
	counts := map[int]SeasonProgress{}
	for _, ep := range episodes {
		if !active[ep.Season] || !ep.Monitored {
			continue
		}
		count := counts[ep.Season]
		count.Season = ep.Season
		count.Total++
		if ep.HasFile {
			count.Imported++
		}
		counts[ep.Season] = count
	}
	return counts
}
func (m *Manager) seasonProgress(ctx context.Context, downloads []models.Download) ([]SeasonProgress, []ServiceError) {
	result := []SeasonProgress{}
	errors := []ServiceError{}
	type groupKey struct{ service, series int64 }
	groups := map[groupKey]map[int64]bool{}
	for _, d := range downloads {
		if d.Source != models.ServiceSonarr || d.SeriesID == 0 || d.EpisodeID == 0 {
			continue
		}
		key := groupKey{d.SourceServiceID, d.SeriesID}
		if groups[key] == nil {
			groups[key] = map[int64]bool{}
		}
		groups[key][d.EpisodeID] = true
	}
	for group, queued := range groups {
		service, err := m.Get(group.service)
		if err != nil || !service.Enabled {
			continue
		}
		credential, err := m.Credential(service.ID)
		if err != nil {
			errors = append(errors, serviceError(service, err))
			continue
		}
		client := integrations.NewHTTPClient()
		arrClient := arr.NewClient("v3")
		endpoint, err := arrClient.Endpoint(service.BaseURL, fmt.Sprintf("episode?seriesId=%d", group.series))
		if err != nil {
			errors = append(errors, serviceError(service, err))
			continue
		}
		var episodes []seasonEpisode
		if err = client.GetJSON(ctx, endpoint, map[string]string{"X-Api-Key": credential}, &episodes); err != nil {
			errors = append(errors, serviceError(service, fmt.Errorf("unable to read Sonarr import progress")))
			continue
		}
		endpoint, err = arrClient.Endpoint(service.BaseURL, fmt.Sprintf("series/%d", group.series))
		if err != nil {
			continue
		}
		var series struct {
			Title string `json:"title"`
		}
		if err = client.GetJSON(ctx, endpoint, map[string]string{"X-Api-Key": credential}, &series); err != nil {
			errors = append(errors, serviceError(service, fmt.Errorf("unable to read Sonarr series details")))
			continue
		}
		for season, count := range countSeasonProgress(episodes, queued) {
			count.ID = fmt.Sprintf("sonarr-season:%d:%d:%d", group.service, group.series, season)
			count.ShowID = fmt.Sprintf("sonarr-show:%d:%d", group.service, group.series)
			count.ShowTitle = series.Title
			count.Title = fmt.Sprintf("%s — Season %d", series.Title, season)
			result = append(result, count)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, errors
}
