package nzbget

import (
	"context"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct{}

func New() *Integration {
	return &Integration{}
}

func (i *Integration) Type() models.ServiceType {
	return models.ServiceNZBGet
}

func (i *Integration) TestConnection(
	ctx context.Context,
	baseURL string,
	credential string,
) (integrations.ConnectionResult, error) {
	auth, err := integrations.DecodeUsernamePassword(credential)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}

	client := NewClient()

	var version string
	if err := client.Call(
		ctx,
		baseURL,
		auth.Username,
		auth.Password,
		"version",
		&version,
	); err != nil {
		return integrations.ConnectionResult{}, err
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Connected to NZBGet",
		Version: version,
	}, nil
}
