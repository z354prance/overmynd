package integrations

import (
	"context"

	"github.com/z354prance/overmynd/internal/models"
)

// MissingProvider is implemented by integrations that can report monitored
// media that is expected to exist but is not currently available.
type MissingProvider interface {
	Missing(
		ctx context.Context,
		service models.Service,
		credential string,
	) ([]models.AttentionItem, error)
}
