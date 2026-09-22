package seerr

import (
	"context"
	"strings"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct{}

type statusResponse struct {
	Version string `json:"version"`
}

func New() *Integration {
	return &Integration{}
}

func (i *Integration) Type() models.ServiceType {
	return models.ServiceSeerr
}

func (i *Integration) TestConnection(
	ctx context.Context,
	baseURL string,
	credential string,
) (integrations.ConnectionResult, error) {
	apiKey := strings.TrimSpace(credential)

	if apiKey == "" {
		return integrations.ConnectionResult{
			OK:      false,
			Message: "Seerr API key is required",
		}, nil
	}

	var status statusResponse

	if err := NewClient().GetJSON(
		ctx,
		baseURL,
		apiKey,
		"status",
		&status,
	); err != nil {
		return integrations.ConnectionResult{}, err
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Connected to Seerr",
		Version: status.Version,
	}, nil
}
