package sonarr

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
	return models.ServiceSonarr
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

	if !strings.EqualFold(status.AppName, "Sonarr") {
		return integrations.ConnectionResult{}, fmt.Errorf(
			"expected Sonarr, service identified as %q",
			status.AppName,
		)
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Connected to Sonarr",
		Version: status.Version,
	}, nil
}
