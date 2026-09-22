package arr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestQueueAndNormalization(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.URL.Path != "/api/v3/queue" {
				t.Fatalf(
					"path = %q, want /api/v3/queue",
					r.URL.Path,
				)
			}

			if got := r.Header.Get("X-Api-Key"); got != "secret" {
				t.Fatalf(
					"X-Api-Key = %q, want secret",
					got,
				)
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = w.Write([]byte(`{
				"page": 1,
				"pageSize": 100,
				"totalRecords": 1,
				"records": [
					{
						"id": 513606121,
						"seriesId": 342,
						"episodeId": 104080,
						"title": "Example S04E10 1080p WEB",
						"status": "completed",
						"trackedDownloadStatus": "warning",
						"protocol": "torrent",
						"downloadClient": "qbittorrent",
						"downloadId": "ABC123",
						"outputPath": "/data/Complete/example",
						"size": 1329513472,
						"sizeleft": 0,
						"timeleft": "00:00:00",
						"estimatedCompletionTime": "2026-09-21T06:45:28Z"
					}
				]
			}`))
		}),
	)
	defer server.Close()

	client := NewClient("v3")

	queue, err := client.Queue(
		context.Background(),
		server.URL,
		"secret",
	)
	if err != nil {
		t.Fatalf("Queue returned error: %v", err)
	}

	if queue.TotalRecords != 1 {
		t.Fatalf(
			"TotalRecords = %d, want 1",
			queue.TotalRecords,
		)
	}

	service := models.Service{
		ID:      2,
		Type:    models.ServiceSonarr,
		Name:    "Sonarr",
		Enabled: true,
		BaseURL: server.URL,
	}

	downloads := NormalizeQueue(service, queue)

	if len(downloads) != 1 {
		t.Fatalf(
			"downloads length = %d, want 1",
			len(downloads),
		)
	}

	got := downloads[0]

	if got.Source != models.ServiceSonarr {
		t.Fatalf(
			"Source = %q, want sonarr",
			got.Source,
		)
	}

	if got.SourceServiceID != 2 {
		t.Fatalf(
			"SourceServiceID = %d, want 2",
			got.SourceServiceID,
		)
	}

	if got.DownloadID != "ABC123" {
		t.Fatalf(
			"DownloadID = %q, want ABC123",
			got.DownloadID,
		)
	}

	if got.SeriesID != 342 {
		t.Fatalf(
			"SeriesID = %d, want 342",
			got.SeriesID,
		)
	}

	if got.EpisodeID != 104080 {
		t.Fatalf(
			"EpisodeID = %d, want 104080",
			got.EpisodeID,
		)
	}

	if got.Size != 1329513472 {
		t.Fatalf(
			"Size = %d, want 1329513472",
			got.Size,
		)
	}

	if got.EstimatedArrival == nil {
		t.Fatal("EstimatedArrival is nil")
	}

	if got.EstimatedArrival.UTC().Format(
		"2006-01-02T15:04:05Z",
	) != "2026-09-21T06:45:28Z" {
		t.Fatalf(
			"EstimatedArrival = %v",
			got.EstimatedArrival,
		)
	}
}
