package lidarr

import (
	"context"

	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
)

func (i *Integration) Queue(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.Download, error) {
	client := arr.NewClient("v1")

	queue, err := client.Queue(
		ctx,
		service.BaseURL,
		credential,
	)
	if err != nil {
		return nil, err
	}

	return arr.NormalizeQueue(service, queue), nil
}
