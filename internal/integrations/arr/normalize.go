package arr

import (
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

func NormalizeQueue(
	service models.Service,
	queue QueueResponse,
) []models.Download {
	downloads := make([]models.Download, 0, len(queue.Records))

	for _, record := range queue.Records {
		download := models.Download{
			ID:              strconv.FormatInt(record.ID, 10),
			Source:          service.Type,
			SourceServiceID: service.ID,
			Title:           record.Title,
			Status:          record.Status,
			TrackedDownload: record.TrackedDownloadStatus,
			Protocol:        record.Protocol,
			DownloadClient:  record.DownloadClient,
			DownloadID:      record.DownloadID,
			OutputPath:      record.OutputPath,
			Size:            record.Size,
			SizeLeft:        record.Sizeleft,
			TimeLeft:        record.Timeleft,
			MovieID:         record.MovieID,
			SeriesID:        record.SeriesID,
			EpisodeID:       record.EpisodeID,
			ArtistID:        record.ArtistID,
			AlbumID:         record.AlbumID,
		}

		if record.EstimatedCompletionTime != "" {
			if parsed, err := time.Parse(
				time.RFC3339,
				record.EstimatedCompletionTime,
			); err == nil {
				download.EstimatedArrival = &parsed
			}
		}

		downloads = append(downloads, download)
	}

	return downloads
}
