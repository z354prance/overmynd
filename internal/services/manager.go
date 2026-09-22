package services

import (
	"context"
	"fmt"
	"strings"

	"github.com/z354prance/overmynd/internal/database"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Manager struct {
	db *database.Database
}

type Input struct {
	Type       models.ServiceType `json:"type"`
	Name       string             `json:"name"`
	Enabled    bool               `json:"enabled"`
	BaseURL    string             `json:"base_url"`
	Credential string             `json:"credential,omitempty"`
}

func NewManager(db *database.Database) *Manager {
	return &Manager{db: db}
}

func (m *Manager) List() ([]models.Service, error) {
	return m.db.ListServices()
}

func (m *Manager) Get(id int64) (models.Service, error) {
	return m.db.GetService(id)
}

func (m *Manager) Credential(id int64) (string, error) {
	return m.db.GetServiceCredential(id)
}

func (m *Manager) Create(input Input) (models.Service, error) {
	normalized, err := validate(input)
	if err != nil {
		return models.Service{}, err
	}

	id, err := m.db.CreateService(database.ServiceInput{
		Type:       normalized.Type,
		Name:       normalized.Name,
		Enabled:    normalized.Enabled,
		BaseURL:    normalized.BaseURL,
		Credential: normalized.Credential,
	})
	if err != nil {
		return models.Service{}, err
	}

	return m.db.GetService(id)
}

func (m *Manager) Update(
	id int64,
	input Input,
	updateCredential bool,
) (models.Service, error) {
	normalized, err := validate(input)
	if err != nil {
		return models.Service{}, err
	}

	err = m.db.UpdateService(
		id,
		database.ServiceInput{
			Type:       normalized.Type,
			Name:       normalized.Name,
			Enabled:    normalized.Enabled,
			BaseURL:    normalized.BaseURL,
			Credential: normalized.Credential,
		},
		updateCredential,
	)
	if err != nil {
		return models.Service{}, err
	}

	return m.db.GetService(id)
}

func (m *Manager) Delete(id int64) error {
	return m.db.DeleteService(id)
}

func validate(input Input) (Input, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Credential = strings.TrimSpace(input.Credential)

	if !supported(input.Type) {
		return Input{}, fmt.Errorf(
			"unsupported service type: %s",
			input.Type,
		)
	}

	if input.Name == "" {
		return Input{}, fmt.Errorf("service name is required")
	}

	if len(input.Name) > 100 {
		return Input{}, fmt.Errorf(
			"service name must be 100 characters or fewer",
		)
	}

	baseURL, err := integrations.NormalizeBaseURL(input.BaseURL)
	if err != nil {
		return Input{}, err
	}

	input.BaseURL = baseURL

	return input, nil
}

func supported(serviceType models.ServiceType) bool {
	for _, candidate := range models.SupportedServices {
		if candidate == serviceType {
			return true
		}
	}

	return false
}

func (m *Manager) TestConnection(
	id int64,
	registry *integrations.Registry,
) (integrations.ConnectionResult, error) {
	service, err := m.db.GetService(id)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}

	integration, ok := registry.Get(service.Type)
	if !ok {
		return integrations.ConnectionResult{}, fmt.Errorf(
			"integration not available for %s",
			service.Type,
		)
	}

	credential, err := m.db.GetServiceCredential(id)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}

	return integration.TestConnection(
		context.Background(),
		service.BaseURL,
		credential,
	)
}
