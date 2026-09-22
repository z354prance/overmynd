package services

import (
	"context"
	"fmt"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type DownloadResult struct {
	Downloads []models.Download `json:"downloads"`
	Errors    []ServiceError    `json:"errors"`
}

type ServiceError struct {
	ServiceID int64              `json:"service_id"`
	Type      models.ServiceType `json:"type"`
	Name      string             `json:"name"`
	Error     string             `json:"error"`
}

func (m *Manager) Downloads(
	ctx context.Context,
	registry *integrations.Registry,
) (DownloadResult, error) {
	services, err := m.db.ListServices()
	if err != nil {
		return DownloadResult{}, err
	}

	result := DownloadResult{
		Downloads: make([]models.Download, 0),
		Errors:    make([]ServiceError, 0),
	}

	for _, service := range services {
		if !service.Enabled {
			continue
		}

		integration, ok := registry.Get(service.Type)
		if !ok {
			continue
		}

		provider, ok := integration.(integrations.QueueProvider)
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

		downloads, err := provider.Queue(
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

		result.Downloads = append(
			result.Downloads,
			downloads...,
		)
	}

	return result, nil
}

func serviceError(
	service models.Service,
	err error,
) ServiceError {
	return ServiceError{
		ServiceID: service.ID,
		Type:      service.Type,
		Name:      service.Name,
		Error:     fmt.Sprintf("%v", err),
	}
}
