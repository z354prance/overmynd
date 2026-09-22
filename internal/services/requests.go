package services

import (
	"context"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type RequestResult struct {
	Requests []models.Request `json:"requests"`
	Errors   []ServiceError   `json:"errors"`
}

func (m *Manager) Requests(
	ctx context.Context,
	registry *integrations.Registry,
) (RequestResult, error) {
	result := RequestResult{
		Requests: []models.Request{},
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

		provider, ok := integration.(integrations.RequestProvider)
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

		requests, err := provider.Requests(
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

		result.Requests = append(
			result.Requests,
			requests...,
		)
	}

	return result, nil
}
