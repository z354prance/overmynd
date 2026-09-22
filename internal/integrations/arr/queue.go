package arr

import (
	"context"
	"fmt"
)

type QueueResponse struct {
	Page          int           `json:"page"`
	PageSize      int           `json:"pageSize"`
	SortKey       string        `json:"sortKey"`
	SortDirection string        `json:"sortDirection"`
	TotalRecords  int           `json:"totalRecords"`
	Records       []QueueRecord `json:"records"`
}

type QueueRecord struct {
	ID                      int64  `json:"id"`
	MovieID                 int64  `json:"movieId"`
	SeriesID                int64  `json:"seriesId"`
	EpisodeID               int64  `json:"episodeId"`
	ArtistID                int64  `json:"artistId"`
	AlbumID                 int64  `json:"albumId"`
	Title                   string `json:"title"`
	Status                  string `json:"status"`
	TrackedDownloadStatus   string `json:"trackedDownloadStatus"`
	Protocol                string `json:"protocol"`
	DownloadClient          string `json:"downloadClient"`
	DownloadID              string `json:"downloadId"`
	OutputPath              string `json:"outputPath"`
	Size                    int64  `json:"size"`
	Sizeleft                int64  `json:"sizeleft"`
	Timeleft                string `json:"timeleft"`
	EstimatedCompletionTime string `json:"estimatedCompletionTime"`
}

func (c *Client) Queue(
	ctx context.Context,
	baseURL string,
	apiKey string,
) (QueueResponse, error) {
	if apiKey == "" {
		return QueueResponse{}, fmt.Errorf("API key is required")
	}

	endpoint, err := c.Endpoint(
		baseURL,
		"queue?page=1&pageSize=100",
	)
	if err != nil {
		return QueueResponse{}, err
	}

	var queue QueueResponse

	err = c.http.GetJSON(
		ctx,
		endpoint,
		map[string]string{
			"X-Api-Key": apiKey,
		},
		&queue,
	)
	if err != nil {
		return QueueResponse{}, err
	}

	return queue, nil
}
