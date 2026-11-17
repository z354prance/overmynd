package tdarr

import (
	"context"
	"strings"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct{}

type statusResponse struct {
	Status       string `json:"status"`
	IsProduction bool   `json:"isProduction"`
	OS           string `json:"os"`
	Version      string `json:"version"`
	BuildDate    string `json:"buildDate"`
	ServerEngine string `json:"serverEngine"`
}

func New() *Integration {
	return &Integration{}
}

func (i *Integration) Type() models.ServiceType {
	return models.ServiceTdarr
}

func (i *Integration) TestConnection(
	ctx context.Context,
	baseURL string,
	credential string,
) (integrations.ConnectionResult, error) {
	var status statusResponse

	if err := NewClient().GetJSON(
		ctx,
		baseURL,
		"status",
		&status,
	); err != nil {
		return integrations.ConnectionResult{
			OK:      false,
			Message: err.Error(),
		}, nil
	}

	if strings.ToLower(status.Status) != "good" {
		return integrations.ConnectionResult{
			OK:      false,
			Message: "Tdarr reported status " + status.Status,
			Version: status.Version,
		}, nil
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Connected to Tdarr",
		Version: status.Version,
	}, nil
}
