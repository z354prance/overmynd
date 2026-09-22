package lidarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestMissing(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.URL.Path != "/api/v1/wanted/missing" {
				t.Fatalf(
					"path = %q, want /api/v1/wanted/missing",
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
				"pageSize": 1000,
				"totalRecords": 1,
				"records": [
					{
						"id": 55,
						"title": "Example Album",
						"monitored": true,
						"releaseDate": "2026-09-20T00:00:00Z",
						"foreignAlbumId": "album-mbid",
						"artist": {
							"id": 12,
							"artistName": "Example Artist",
							"foreignArtistId": "artist-mbid"
						}
					}
				]
			}`))
		}),
	)
	defer server.Close()

	integration := New()

	service := models.Service{
		ID:      3,
		Type:    models.ServiceLidarr,
		Name:    "Lidarr",
		Enabled: true,
		BaseURL: server.URL,
	}

	items, err := integration.Missing(
		context.Background(),
		service,
		"secret",
	)
	if err != nil {
		t.Fatalf("Missing returned error: %v", err)
	}

	if len(items) != 1 {
		t.Fatalf(
			"items length = %d, want 1",
			len(items),
		)
	}

	got := items[0]

	if got.Kind != models.MediaAlbum {
		t.Fatalf(
			"Kind = %q, want album",
			got.Kind,
		)
	}

	if got.Title != "Example Artist - Example Album" {
		t.Fatalf(
			"Title = %q",
			got.Title,
		)
	}

	if got.MusicBrainzID != "album-mbid" {
		t.Fatalf(
			"MusicBrainzID = %q, want album-mbid",
			got.MusicBrainzID,
		)
	}

	if got.AirDate == nil {
		t.Fatal("AirDate is nil")
	}
}
