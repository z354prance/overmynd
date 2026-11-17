package integrations

import (
	"context"

	"github.com/z354prance/overmynd/internal/models"
)

type PlaybackProvider interface {
	Playback(
		ctx context.Context,
		service models.Service,
		credential string,
	) ([]models.PlaybackSession, error)
}
