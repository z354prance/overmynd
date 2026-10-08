package services

import (
	"context"
	"fmt"

	"github.com/z354prance/overmynd/internal/correlation"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type ActivityResult struct {
	Seasons    []SeasonProgress        `json:"seasons"`
	Lifecycles []models.MediaLifecycle `json:"lifecycles"`
	Errors     []ServiceError          `json:"errors"`
}

func (m *Manager) Activity(
	ctx context.Context,
	registry *integrations.Registry,
) (ActivityResult, error) {
	result := ActivityResult{
		Lifecycles: []models.MediaLifecycle{},
		Errors:     []ServiceError{},
	}

	missing, err := m.Missing(ctx, registry)
	if err != nil {
		return ActivityResult{}, err
	}
	result.Errors = append(result.Errors, missing.Errors...)

	requests, err := m.Requests(ctx, registry)
	if err != nil {
		return ActivityResult{}, err
	}
	result.Errors = append(result.Errors, requests.Errors...)

	downloads, err := m.Downloads(ctx, registry)
	if err != nil {
		return ActivityResult{}, err
	}
	result.Errors = append(result.Errors, downloads.Errors...)

	processing, err := m.Processing(ctx, registry)
	if err != nil {
		return ActivityResult{}, err
	}
	result.Errors = append(result.Errors, processing.Errors...)

	playback, err := m.Playback(ctx, registry)
	if err != nil {
		return ActivityResult{}, err
	}
	result.Errors = append(result.Errors, playback.Errors...)

	seasons, seasonErrors := m.seasonProgress(ctx, downloads.Downloads, processing.Jobs...)
	result.Seasons = seasons
	result.Errors = append(result.Errors, seasonErrors...)

	engine := correlation.New()

	result.Lifecycles = engine.Build(correlation.Input{
		Attention:  missing.Items,
		Requests:   requests.Requests,
		Downloads:  downloads.Downloads,
		Processing: processing.Jobs,
		Playback:   playback.Sessions,
	})

	for i := range result.Lifecycles {
		item := &result.Lifecycles[i]
		for _, ref := range item.References {
			if ref.RecordType != "download" {
				continue
			}
			for _, d := range downloads.Downloads {
				if ref.Source != d.Source || ref.SourceServiceID != d.SourceServiceID || ref.RecordID != d.ID {
					continue
				}
				id := int64(0)
				if d.Source == models.ServiceSonarr {
					id = d.SeriesID
				}
				if d.Source == models.ServiceRadarr {
					id = d.MovieID
				}
				if id > 0 {
					item.PosterURL = fmt.Sprintf("/MediaCover/%d/poster.jpg", id)
					item.PosterServiceID = d.SourceServiceID
				}
			}
		}
	}
	return result, nil
}
