package services

import (
	"context"

	"github.com/z354prance/overmynd/internal/correlation"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type ActivityResult struct {
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

	engine := correlation.New()

	result.Lifecycles = engine.Build(correlation.Input{
		Attention:  missing.Items,
		Requests:   requests.Requests,
		Downloads:  downloads.Downloads,
		Processing: processing.Jobs,
		Playback:   playback.Sessions,
	})

	return result, nil
}
