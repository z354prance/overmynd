package radarr

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
						"id": 42,
						"title": "Example Movie",
						"year": 2026,
						"monitored": true,
						"tmdbId": 123456,
						"imdbId": "tt1234567",
						"digitalRelease": "2026-09-20T00:00:00Z"
					}
				]
			}`))
		}),
	)
	defer server.Close()

	integration := New()

	service := models.Service{
		ID:      1,
		Type:    models.ServiceRadarr,
		Name:    "Radarr",
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

	if got.State != models.AttentionMissing {
		t.Fatalf(
			"State = %q, want missing",
			got.State,
		)
	}

	if got.Kind != models.MediaMovie {
		t.Fatalf(
			"Kind = %q, want movie",
			got.Kind,
		)
	}

	if got.SourceID != 42 {
		t.Fatalf(
			"SourceID = %d, want 42",
			got.SourceID,
		)
	}

	if got.TMDBID != 123456 {
		t.Fatalf(
			"TMDBID = %d, want 123456",
			got.TMDBID,
		)
	}

	if got.AirDate == nil {
		t.Fatal("AirDate is nil")
	}
}
