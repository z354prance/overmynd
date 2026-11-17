package integrations

import (
	"context"

	"github.com/z354prance/overmynd/internal/models"
)

// QueueProvider is implemented by integrations that expose downloadable
// queue items. It is intentionally separate from Integration so services
// without queues do not need dummy implementations.
type QueueProvider interface {
	Queue(
		ctx context.Context,
		service models.Service,
		credential string,
	) ([]models.Download, error)
}
