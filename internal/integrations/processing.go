package integrations

import (
	"context"

	"github.com/z354prance/overmynd/internal/models"
)

type ProcessingProvider interface {
	Processing(ctx context.Context, service models.Service, credential string) ([]models.ProcessingJob, error)
}
