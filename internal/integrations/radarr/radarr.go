package radarr

import (
	"context"
	"fmt"
	"strings"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct {
	client *arr.Client
}

func New() *Integration {
	return &Integration{
		client: arr.NewClient("v3"),
	}
}

func (i *Integration) Type() models.ServiceType {
	return models.ServiceRadarr
}

func (i *Integration) TestConnection(
	ctx context.Context,
	baseURL string,
	credential string,
) (integrations.ConnectionResult, error) {
	status, err := i.client.SystemStatus(
		ctx,
		baseURL,
		credential,
	)
	if err != nil {
		return integrations.ConnectionResult{}, err
	}

	if !strings.EqualFold(status.AppName, "Radarr") {
		return integrations.ConnectionResult{}, fmt.Errorf(
			"expected Radarr, service identified as %q",
			status.AppName,
		)
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Connected to Radarr",
		Version: status.Version,
	}, nil
}
