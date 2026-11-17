package integrations

import (
	"context"

	"github.com/z354prance/overmynd/internal/models"
)

type ConnectionResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
	Version string `json:"version,omitempty"`
}

type Integration interface {
	Type() models.ServiceType

	TestConnection(
		ctx context.Context,
		baseURL string,
		credential string,
	) (ConnectionResult, error)
}
