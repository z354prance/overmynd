package services

import (
	"context"
	"fmt"
	"github.com/z354prance/overmynd/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSeasonProgressCountsImportedMonitoredEpisodes(t *testing.T) {
	episodes := []seasonEpisode{{ID: 1, Season: 1, Monitored: true, HasFile: true}, {ID: 2, Season: 1, Monitored: true}, {ID: 3, Season: 1, Monitored: false, HasFile: true}, {ID: 4, Season: 2, Monitored: true}}
	got := countSeasonProgress(episodes, map[int64]bool{2: true})
	if len(got) != 2 || got[1].Imported != 1 || got[1].Total != 2 {
		t.Fatal(got)
	}
	episodes[1].HasFile = true
	got = countSeasonProgress(episodes, map[int64]bool{2: true})
	if got[1].Imported != 2 {
		t.Fatal(got)
	}
	if len(countSeasonProgress(episodes, map[int64]bool{})) != 0 {
		t.Fatal("inactive seasons shown")
	}
}

func TestSeasonProgressReadsSonarrFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "secret" {
			t.Error("missing credential")
		}
		switch r.URL.Path {
		case "/api/v3/episode":
			if r.URL.Query().Get("seriesId") != "24" {
				t.Error("wrong series")
			}
			fmt.Fprint(w, `[{"id":1,"seasonNumber":1,"monitored":true,"hasFile":true},{"id":2,"seasonNumber":1,"monitored":true,"hasFile":false}]`)
		case "/api/v3/series/24":
			fmt.Fprint(w, `{"title":"24"}`)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	m := testManager(t)
	service, err := m.Create(Input{Type: models.ServiceSonarr, Name: "Sonarr", Enabled: true, BaseURL: server.URL, Credential: "secret"})
	if err != nil {
		t.Fatal(err)
	}
	items, errs := m.seasonProgress(context.Background(), []models.Download{{Source: models.ServiceSonarr, SourceServiceID: service.ID, SeriesID: 24, EpisodeID: 2, Size: 100, SizeLeft: 0}})
	if len(errs) != 0 || len(items) != 1 || items[0].Imported != 1 || items[0].Total != 2 || items[0].Title != "24 — Season 1" {
		t.Fatal(items, errs)
	}
}

func TestActiveShowIncludesCompletedAndNonQueuedSeasons(t *testing.T) {
	episodes := []seasonEpisode{}
	for season := 1; season <= 9; season++ {
		episodes = append(episodes, seasonEpisode{ID: int64(season), Season: season, Monitored: true, HasFile: season == 1})
	}
	// Only season 9 has a queue record in this snapshot.
	got := countSeasonProgress(episodes, map[int64]bool{9: true})
	if len(got) != 9 || got[1].Imported != 1 || got[1].Total != 1 {
		t.Fatalf("completed season missing: %+v", got)
	}
	for season := 2; season <= 9; season++ {
		if got[season].Total != 1 || got[season].Imported != 0 {
			t.Fatalf("season %d missing: %+v", season, got)
		}
	}
}

func TestSeriesOverviewPersistsUntilSonarrImportsAllEpisodes(t *testing.T) {
	complete := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v3/series":
			fmt.Fprint(w, `[{"id":24,"title":"MacGyver (2016)","year":2016}]`)
		case "/api/v3/series/24":
			fmt.Fprint(w, `{"title":"MacGyver (2016)"}`)
		case "/api/v3/episode":
			fmt.Fprintf(w, `[{"id":1,"seasonNumber":1,"monitored":true,"hasFile":true},{"id":2,"seasonNumber":2,"monitored":true,"hasFile":%t}]`, complete)
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	m := testManager(t)
	if _, err := m.Create(Input{Type: models.ServiceSonarr, Name: "Sonarr", Enabled: true, BaseURL: server.URL, Credential: "secret"}); err != nil {
		t.Fatal(err)
	}
	pending := []models.ProcessingJob{{Source: models.ServiceTdarr, State: models.ProcessingStateQueued, Title: "MacGyver.2016.S02E03.1080p.mkv"}}
	if items, errs := m.seasonProgress(context.Background(), nil, pending...); len(items) != 0 || len(errs) != 0 {
		t.Fatal("pending show was enrolled", items, errs)
	}
	jobs := []models.ProcessingJob{{Source: models.ServiceTdarr, State: models.ProcessingStateProcessing, Title: "MacGyver.2016.S02E03.1080p.mkv"}}
	items, errs := m.seasonProgress(context.Background(), nil, jobs...)
	if len(errs) != 0 || len(items) != 2 {
		t.Fatal(items, errs)
	}
	// Recreate manager: tracking must survive a restart and empty queues.
	m = NewManager(m.db)
	items, errs = m.seasonProgress(context.Background(), nil)
	if len(errs) != 0 || len(items) != 2 {
		t.Fatal("lost unfinished show", items, errs)
	}
	complete = true
	items, errs = m.seasonProgress(context.Background(), nil)
	if len(errs) != 0 || len(items) != 0 {
		t.Fatal("completed show retained", items, errs)
	}
	complete = false
	items, errs = m.seasonProgress(context.Background(), nil)
	if len(errs) != 0 || len(items) != 0 {
		t.Fatal("inactive missing show reappeared", items, errs)
	}
}
