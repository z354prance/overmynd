package qbittorrent

import (
	"context"
	"fmt"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct{}

func New() *Integration {
	return &Integration{}
}

func (i *Integration) Type() models.ServiceType {
	return models.ServiceQBittorrent
}

func (i *Integration) TestConnection(
	ctx context.Context,
	baseURL string,
	credential string,
) (integrations.ConnectionResult, error) {
	auth, err := integrations.DecodeUsernamePassword(
		credential,
	)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}

	client := NewClient()

	if err := client.Login(
		ctx,
		baseURL,
		auth.Username,
		auth.Password,
	); err != nil {
		return integrations.ConnectionResult{}, err
	}

	version, err := client.GetText(
		ctx,
		baseURL,
		"/api/v2/app/version",
	)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}

	if version == "" {
		return integrations.ConnectionResult{},
			fmt.Errorf("qBittorrent returned an empty version")
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Connected to qBittorrent",
		Version: version,
	}, nil
}
