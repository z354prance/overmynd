package qbittorrent

import (
	"context"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

type torrent struct {
	Hash          string  `json:"hash"`
	Name          string  `json:"name"`
	State         string  `json:"state"`
	Progress      float64 `json:"progress"`
	Size          int64   `json:"size"`
	Downloaded    int64   `json:"downloaded"`
	AmountLeft    int64   `json:"amount_left"`
	ETA           int64   `json:"eta"`
	SavePath      string  `json:"save_path"`
	ContentPath   string  `json:"content_path"`
	DownloadSpeed int64   `json:"dlspeed"`
	UploadSpeed   int64   `json:"upspeed"`
	Ratio         float64 `json:"ratio"`
	AddedOn       int64   `json:"added_on"`
	CompletionOn  int64   `json:"completion_on"`
}

func (i *Integration) Queue(
	ctx context.Context,
	service models.Service,
	credential string,
) ([]models.Download, error) {
	auth, err := integrations.DecodeUsernamePassword(credential)
	if err != nil {
		return nil, err
	}

	client := NewClient()

	err = client.Login(
		ctx,
		service.BaseURL,
		auth.Username,
		auth.Password,
	)
	if err != nil {
		return nil, err
	}

	var torrents []torrent

	err = client.GetJSON(
		ctx,
		service.BaseURL,
		"/api/v2/torrents/info?filter=all",
		&torrents,
	)
	if err != nil {
		return nil, err
	}

	items := make([]models.Download, 0, len(torrents))

	for _, t := range torrents {
		if isFinishedSeeder(t) {
			continue
		}

		outputPath := t.ContentPath
		if outputPath == "" {
			outputPath = t.SavePath
		}

		item := models.Download{
			ID:              strconv.FormatInt(service.ID, 10) + ":torrent:" + t.Hash,
			Source:          service.Type,
			SourceServiceID: service.ID,
			Title:           t.Name,
			Status:          t.State,
			Protocol:        "torrent",
			DownloadClient:  service.Name,
			DownloadID:      t.Hash,
			OutputPath:      outputPath,
			Size:            t.Size,
			SizeLeft:        t.AmountLeft,
		}

		if t.ETA > 0 && t.ETA < 8640000 {
			duration := time.Duration(t.ETA) * time.Second
			item.TimeLeft = duration.String()
		}

		items = append(items, item)
	}

	return items, nil
}

func isFinishedSeeder(t torrent) bool {
	if t.Progress < 1 {
		return false
	}

	switch t.State {
	case "uploading",
		"stalledUP",
		"queuedUP",
		"forcedUP",
		"pausedUP",
		"stoppedUP":
		return true
	default:
		return false
	}
}
