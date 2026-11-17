package tracearr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestPlaybackNormalizesStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v2/public/streams" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}

			if got := r.Header.Get("Authorization"); got != "Bearer trr_pub_test" {
				t.Fatalf("unexpected authorization: %q", got)
			}

			w.Header().Set("Content-Type", "application/json")

			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{
						"id":                "session-1",
						"server_id":         "server-1",
						"server_name":       "Emby",
						"server_type":       "emby",
						"username":          "viewer",
						"media_title":       "Pilot",
						"media_type":        "episode",
						"show_title":        "Example Show",
						"season_number":     1,
						"episode_number":    1,
						"year":              2026,
						"duration_ms":       3600000,
						"progress_ms":       900000,
						"state":             "playing",
						"started_at":        "2026-09-21T17:00:00Z",
						"is_transcode":      true,
						"video_decision":    "transcode",
						"audio_decision":    "directplay",
						"bitrate":           8000000,
						"device":            "Browser",
						"player":            "Web",
						"product":           "Emby",
						"platform":          "Firefox",
						"media_id":          "media-1",
						"show_media_id":     "show-1",
						"imdb_id":           "tt1234567",
						"tmdb_id":           "12345",
						"tvdb_id":           "67890",
						"rating_key":        "rating-1",
						"parent_rating_key": "parent-1",
						"library_id":        "library-1",
						"poster_url":        "/poster.jpg",
						"genres":            []string{"Drama"},
					},
				},
			})
		},
	))
	defer server.Close()

	integration := New()

	sessions, err := integration.Playback(
		context.Background(),
		models.Service{
			ID:      8,
			Type:    models.ServiceTracearr,
			BaseURL: server.URL,
		},
		"trr_pub_test",
	)
	if err != nil {
		t.Fatalf("Playback returned error: %v", err)
	}

	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}

	got := sessions[0]

	if got.ID != "session-1" {
		t.Fatalf("unexpected ID: %q", got.ID)
	}

	if got.Source != models.ServiceTracearr {
		t.Fatalf("unexpected source: %q", got.Source)
	}

	if got.SourceServiceID != 8 {
		t.Fatalf(
			"unexpected source service ID: %d",
			got.SourceServiceID,
		)
	}

	if got.MediaTitle != "Pilot" {
		t.Fatalf("unexpected media title: %q", got.MediaTitle)
	}

	if got.ShowTitle != "Example Show" {
		t.Fatalf("unexpected show title: %q", got.ShowTitle)
	}

	if got.SeasonNumber != 1 || got.EpisodeNumber != 1 {
		t.Fatalf(
			"unexpected episode: S%02dE%02d",
			got.SeasonNumber,
			got.EpisodeNumber,
		)
	}

	if got.TMDBID != "12345" || got.TVDBID != "67890" {
		t.Fatalf(
			"unexpected external IDs: TMDB=%q TVDB=%q",
			got.TMDBID,
			got.TVDBID,
		)
	}

	if !got.IsTranscode {
		t.Fatal("expected transcode")
	}

	if got.StartedAt == nil {
		t.Fatal("expected started_at to be parsed")
	}
}
