package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/models"
)

func TestQueue(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			switch r.URL.Path {
			case "/api/v2/auth/login":
				http.SetCookie(
					w,
					&http.Cookie{
						Name:  "SID",
						Value: "test-session",
						Path:  "/",
					},
				)

				_, _ = w.Write([]byte("Ok."))

			case "/api/v2/torrents/info":
				cookie, err := r.Cookie("SID")
				if err != nil {
					t.Fatalf(
						"SID cookie missing: %v",
						err,
					)
				}

				if cookie.Value != "test-session" {
					t.Fatalf(
						"SID = %q",
						cookie.Value,
					)
				}

				w.Header().Set(
					"Content-Type",
					"application/json",
				)

				_, _ = w.Write([]byte(`[
					{
						"hash": "ABC123",
						"name": "Example Download",
						"state": "downloading",
						"progress": 0.5,
						"size": 1000000,
						"downloaded": 500000,
						"amount_left": 500000,
						"eta": 120,
						"save_path": "/downloads/",
						"content_path": "/downloads/example"
					},
					{
						"hash": "DONE123",
						"name": "Finished Seed",
						"state": "uploading",
						"progress": 1,
						"size": 2000000,
						"downloaded": 2000000,
						"amount_left": 0,
						"eta": 8640000,
						"save_path": "/downloads/"
					}
				]`))

			default:
				http.NotFound(w, r)
			}
		}),
	)
	defer server.Close()

	credential, err := integrations.EncodeUsernamePassword(
		"admin",
		"secret",
	)
	if err != nil {
		t.Fatalf("encode credential: %v", err)
	}

	service := models.Service{
		ID:      4,
		Type:    models.ServiceQBittorrent,
		Name:    "qBittorrent",
		Enabled: true,
		BaseURL: server.URL,
	}

	items, err := New().Queue(
		context.Background(),
		service,
		credential,
	)
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf(
			"items length = %d, want 1",
			len(items),
		)
	}

	got := items[0]

	if got.DownloadID != "ABC123" {
		t.Fatalf(
			"DownloadID = %q, want ABC123",
			got.DownloadID,
		)
	}

	if got.Source != models.ServiceQBittorrent {
		t.Fatalf(
			"Source = %q, want qbittorrent",
			got.Source,
		)
	}

	if got.Size != 1000000 {
		t.Fatalf(
			"Size = %d, want 1000000",
			got.Size,
		)
	}

	if got.SizeLeft != 500000 {
		t.Fatalf(
			"SizeLeft = %d, want 500000",
			got.SizeLeft,
		)
	}

	if got.TimeLeft != "2m0s" {
		t.Fatalf(
			"TimeLeft = %q, want 2m0s",
			got.TimeLeft,
		)
	}
}

func TestIsFinishedSeederStoppedUP(t *testing.T) {
	item := torrent{
		State:    "stoppedUP",
		Progress: 1,
	}

	if !isFinishedSeeder(item) {
		t.Fatal("stoppedUP completed torrent should be filtered")
	}
}

func TestIsFinishedSeederStoppedUPIncomplete(t *testing.T) {
	item := torrent{
		State:    "stoppedUP",
		Progress: 0.75,
	}

	if isFinishedSeeder(item) {
		t.Fatal("incomplete stoppedUP torrent should not be filtered")
	}
}
