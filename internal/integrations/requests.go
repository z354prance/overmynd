package integrations

import (
	"context"

	"github.com/z354prance/overmynd/internal/models"
)

type RequestProvider interface {
	Requests(
		ctx context.Context,
		service models.Service,
		credential string,
	) ([]models.Request, error)
}
