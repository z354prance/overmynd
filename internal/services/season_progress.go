package services

import (
	"context"
	"fmt"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
	"sort"
	"strings"
)

type SeasonProgress struct {
	PosterURL       string `json:"poster_url,omitempty"`
	PosterServiceID int64  `json:"-"`
	ShowID          string `json:"show_id"`
	ShowTitle       string `json:"show_title"`
	ID              string `json:"id"`
	Title           string `json:"title"`
	Season          int    `json:"season_number"`
	Imported        int    `json:"imported"`
	Total           int    `json:"total"`
}
type seasonEpisode struct {
	ID        int64 `json:"id"`
	Season    int   `json:"seasonNumber"`
	Monitored bool  `json:"monitored"`
	HasFile   bool  `json:"hasFile"`
}

func countSeasonProgress(episodes []seasonEpisode, queued map[int64]bool) map[int]SeasonProgress {
	active := queued[0] // A matched Tdarr job keeps the series active after its queue entries leave Sonarr.
	for _, ep := range episodes {
		if queued[ep.ID] {
			active = true
		}
	}
	counts := map[int]SeasonProgress{}
	for _, ep := range episodes {
		if !active || !ep.Monitored {
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
func (m *Manager) seasonProgress(ctx context.Context, downloads []models.Download, processing ...models.ProcessingJob) ([]SeasonProgress, []ServiceError) {
	result := []SeasonProgress{}
	errors := []ServiceError{}
	processingShows, processingErrors := m.processingShows(ctx, processing)
	errors = append(errors, processingErrors...)
	downloads = append(append([]models.Download{}, downloads...), processingShows...)
	type groupKey struct{ service, series int64 }
	groups := map[groupKey]map[int64]bool{}
	for _, d := range downloads {
		if d.Source != models.ServiceSonarr || d.SeriesID == 0 {
			continue
		}
		switch strings.ToLower(d.Status) {
		case "queued", "paused", "pending", "waiting":
			if d.SizeLeft >= d.Size {
				continue
			}
		}
		key := groupKey{d.SourceServiceID, d.SeriesID}
		if groups[key] == nil {
			groups[key] = map[int64]bool{}
		}
		groups[key][d.EpisodeID] = true
	}

	// Persist only shows that have actually entered the pipeline, not every
	// missing series. Each key is independent so concurrent refreshes cannot
	// overwrite the set of tracked shows.
	for group := range groups {
		_, err := m.db.DB.ExecContext(ctx, "INSERT INTO settings (key,value,updated_at) VALUES (?, '1', CURRENT_TIMESTAMP) ON CONFLICT(key) DO NOTHING", fmt.Sprintf("tracked-show:%d:%d", group.service, group.series))
		if err != nil {
			return result, append(errors, ServiceError{Error: "unable to save series tracking"})
		}
	}
	rows, err := m.db.DB.QueryContext(ctx, "SELECT key FROM settings WHERE key LIKE 'tracked-show:%'")
	if err != nil {
		return result, append(errors, ServiceError{Error: "unable to read series tracking"})
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			continue
		}
		var group groupKey
		if _, err := fmt.Sscanf(key, "tracked-show:%d:%d", &group.service, &group.series); err == nil {
			if groups[group] == nil {
				groups[group] = map[int64]bool{}
			}
			groups[group][0] = true
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, append(errors, ServiceError{Error: "unable to read series tracking"})
	}
	rows.Close()
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

		counts := countSeasonProgress(episodes, queued)
		complete := len(counts) > 0
		for _, count := range counts {
			if count.Imported < count.Total {
				complete = false
			}
		}
		if complete {
			if _, err := m.db.DB.ExecContext(ctx, "DELETE FROM settings WHERE key = ?", fmt.Sprintf("tracked-show:%d:%d", group.service, group.series)); err != nil {
				errors = append(errors, serviceError(service, fmt.Errorf("unable to finish series tracking")))
			}
			continue
		}
		for season, count := range counts {
			count.ID = fmt.Sprintf("sonarr-season:%d:%d:%d", group.service, group.series, season)
			count.ShowID = fmt.Sprintf("sonarr-show:%d:%d", group.service, group.series)
			count.ShowTitle = series.Title
			count.PosterServiceID = group.service
			count.PosterURL = fmt.Sprintf("/MediaCover/%d/poster.jpg", group.series)
			count.Title = fmt.Sprintf("%s — Season %d", series.Title, season)
			result = append(result, count)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result, errors
}
