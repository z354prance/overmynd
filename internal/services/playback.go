package services

import (
	"context"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type PlaybackResult struct {
	Sessions []models.PlaybackSession `json:"sessions"`
	Errors   []ServiceError           `json:"errors"`
}

func (m *Manager) Playback(
	ctx context.Context,
	registry *integrations.Registry,
) (PlaybackResult, error) {
	result := PlaybackResult{
		Sessions: []models.PlaybackSession{},
		Errors:   []ServiceError{},
	}

	services, err := m.List()
	if err != nil {
		return result, err
	}

	for _, service := range services {
		if !service.Enabled {
			continue
		}

		integration, ok := registry.Get(service.Type)
		if !ok {
			continue
		}

		provider, ok := integration.(integrations.PlaybackProvider)
		if !ok {
			continue
		}

		credential, err := m.Credential(service.ID)
		if err != nil {
			result.Errors = append(
				result.Errors,
				serviceError(service, err),
			)
			continue
		}

		sessions, err := provider.Playback(
			ctx,
			service,
			credential,
		)
		if err != nil {
			result.Errors = append(
				result.Errors,
				serviceError(service, err),
			)
			continue
		}

		result.Sessions = append(
			result.Sessions,
			sessions...,
		)
	}

	return result, nil
}
