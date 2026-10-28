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

	var result QueueResponse
	seen := map[int64]bool{}
	received := 0
	for page := 1; page <= 1000; page++ {
		endpoint, err := c.Endpoint(baseURL, fmt.Sprintf("queue?page=%d&pageSize=100&sortKey=id&sortDirection=ascending", page))
		if err != nil {
			return QueueResponse{}, err
		}
		var batch QueueResponse
		if err := c.http.GetJSON(ctx, endpoint, map[string]string{"X-Api-Key": apiKey}, &batch); err != nil {
			return QueueResponse{}, err
		}
		if page == 1 {
			result = batch
			result.Records = []QueueRecord{}
		}
		added := 0
		for _, record := range batch.Records {
			if !seen[record.ID] {
				seen[record.ID] = true
				result.Records = append(result.Records, record)
				added++
			}
		}
		received += len(batch.Records)
		result.TotalRecords = batch.TotalRecords
		// An empty page is valid if the live queue shrank during pagination.
		if len(batch.Records) == 0 || received >= batch.TotalRecords {
			return result, nil
		}
		if added == 0 {
			return QueueResponse{}, fmt.Errorf("queue pagination made no progress")
		}
	}
	return QueueResponse{}, fmt.Errorf("queue pagination exceeded limit")
}
