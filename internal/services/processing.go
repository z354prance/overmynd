package services

import (
	"context"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type ProcessingResult struct {
	Jobs   []models.ProcessingJob `json:"jobs"`
	Errors []ServiceError         `json:"errors"`
}

func (m *Manager) Processing(
	ctx context.Context,
	registry *integrations.Registry,
) (ProcessingResult, error) {
	result := ProcessingResult{
		Jobs:   []models.ProcessingJob{},
		Errors: []ServiceError{},
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

		provider, ok := integration.(integrations.ProcessingProvider)
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

		jobs, err := provider.Processing(
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

		result.Jobs = append(
			result.Jobs,
			jobs...,
		)
	}

	return result, nil
}
