package tracearr

import (
	"context"
	"fmt"
	"strings"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type Integration struct {
	client *Client
}

type streamsSummaryResponse struct {
	Summary *struct {
		Total int `json:"total"`
	} `json:"summary,omitempty"`
}

func New() *Integration {
	return &Integration{
		client: NewClient(),
	}
}

func (i *Integration) Type() models.ServiceType {
	return models.ServiceTracearr
}

func (i *Integration) TestConnection(
	ctx context.Context,
	baseURL string,
	credential string,
) (integrations.ConnectionResult, error) {
	if strings.TrimSpace(credential) == "" {
		return integrations.ConnectionResult{}, fmt.Errorf(
			"Tracearr Public API key is required",
		)
	}

	var response streamsSummaryResponse

	if err := i.client.GetJSON(
		ctx,
		baseURL,
		"streams?summary=true",
		credential,
		&response,
	); err != nil {
		return integrations.ConnectionResult{}, err
	}

	return integrations.ConnectionResult{
		OK:      true,
		Message: "Tracearr Public API connected",
	}, nil
}
