package arr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
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

func TestQueueReadsAll276Records(t *testing.T) {
	pages := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		pages++
		if r.URL.Query().Get("pageSize") != "100" || r.Header.Get("X-Api-Key") != "secret" {
			t.Error("invalid paginated request")
		}
		batch := QueueResponse{Page: page, PageSize: 100, TotalRecords: 276}
		for n := (page-1)*100 + 1; n <= page*100 && n <= 276; n++ {
			series := int64(1)
			if n > 200 {
				series = 24
			}
			batch.Records = append(batch.Records, QueueRecord{ID: int64(n), SeriesID: series, EpisodeID: int64(n)})
		}
		json.NewEncoder(w).Encode(batch)
	}))
	defer server.Close()
	queue, err := NewClient("v3").Queue(context.Background(), server.URL, "secret")
	if err != nil || len(queue.Records) != 276 || pages != 3 {
		t.Fatalf("records=%d pages=%d err=%v", len(queue.Records), pages, err)
	}
	if queue.Records[275].SeriesID != 24 {
		t.Fatal("later-page show missing")
	}
}

func TestQueuePaginationFailureAndRepeatedPage(t *testing.T) {
	for _, mode := range []string{"failure", "repeat", "empty"} {
		t.Run(mode, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls > 1 && mode == "failure" {
					w.WriteHeader(401)
					return
				}
				batch := QueueResponse{TotalRecords: 276, Records: []QueueRecord{{ID: 1}}}
				if calls > 1 && mode == "empty" {
					batch.Records = nil
				}
				json.NewEncoder(w).Encode(batch)
			}))
			defer server.Close()
			queue, err := NewClient("v3").Queue(context.Background(), server.URL, "secret")
			if calls != 2 {
				t.Fatalf("unexpected calls: %d", calls)
			}
			if mode == "empty" {
				if err != nil || len(queue.Records) != 1 {
					t.Fatal(queue, err)
				}
			} else if err == nil || len(queue.Records) != 0 {
				t.Fatal("partial/repeated result returned as complete", queue, err)
			}
		})
	}
}
