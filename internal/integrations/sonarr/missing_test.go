package sonarr

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
			if r.URL.Path != "/api/v3/wanted/missing" {
				t.Fatalf(
					"path = %q, want /api/v3/wanted/missing",
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
						"id": 104080,
						"seriesId": 342,
						"title": "Episode Title",
						"seasonNumber": 4,
						"episodeNumber": 10,
						"airDateUtc": "2026-09-20T01:00:00Z",
						"monitored": true,
						"series": {
							"title": "Example Series",
							"year": 2023,
							"tvdbId": 12345,
							"imdbId": "tt1234567"
						}
					}
				]
			}`))
		}),
	)
	defer server.Close()

	integration := New()

	service := models.Service{
		ID:      2,
		Type:    models.ServiceSonarr,
		Name:    "Sonarr",
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

	if got.Kind != models.MediaEpisode {
		t.Fatalf(
			"Kind = %q, want episode",
			got.Kind,
		)
	}

	if got.Title != "Example Series" {
		t.Fatalf(
			"Title = %q, want Example Series",
			got.Title,
		)
	}

	if got.SeasonNumber != 4 {
		t.Fatalf(
			"SeasonNumber = %d, want 4",
			got.SeasonNumber,
		)
	}

	if got.EpisodeNumber != 10 {
		t.Fatalf(
			"EpisodeNumber = %d, want 10",
			got.EpisodeNumber,
		)
	}

	if got.TVDBID != 12345 {
		t.Fatalf(
			"TVDBID = %d, want 12345",
			got.TVDBID,
		)
	}

	if got.AirDate == nil {
		t.Fatal("AirDate is nil")
	}
}
