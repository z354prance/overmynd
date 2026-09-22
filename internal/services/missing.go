package services

import (
	"context"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type MissingResult struct {
	Items  []models.AttentionItem `json:"items"`
	Errors []ServiceError         `json:"errors"`
}

func (m *Manager) Missing(
	ctx context.Context,
	registry *integrations.Registry,
) (MissingResult, error) {
	services, err := m.db.ListServices()
	if err != nil {
		return MissingResult{}, err
	}

	result := MissingResult{
		Items:  make([]models.AttentionItem, 0),
		Errors: make([]ServiceError, 0),
	}

	for _, service := range services {
		if !service.Enabled {
			continue
		}

		integration, ok := registry.Get(service.Type)
		if !ok {
			continue
		}

		provider, ok := integration.(integrations.MissingProvider)
		if !ok {
			continue
		}

		credential, err := m.db.GetServiceCredential(service.ID)
		if err != nil {
			result.Errors = append(
				result.Errors,
				serviceError(service, err),
			)
			continue
		}

		items, err := provider.Missing(
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

		result.Items = append(
			result.Items,
			items...,
		)
	}

	return result, nil
}
