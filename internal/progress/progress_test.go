package progress_test

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"context"

	"github.com/z354prance/overmynd/internal/arr"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/progress"
	"github.com/z354prance/overmynd/internal/secret"
	"github.com/z354prance/overmynd/internal/store"
)

var now = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func TestCountSeasons(t *testing.T) {
	old := "2020-01-01T00:00:00Z"
	eps := []arr.Episode{
		{ID: 1, Season: 1, Monitored: true, HasFile: true},
		{ID: 2, Season: 1, Monitored: true, AirDateUTC: old},
		{ID: 3, Season: 1, Monitored: false, HasFile: true},
		{ID: 4, Season: 2, Monitored: true, AirDateUTC: old},
		{ID: 5, Season: 2, Monitored: true},                                     // undated: not missing
		{ID: 6, Season: 2, Monitored: true, AirDateUTC: "2030-01-01T00:00:00Z"}, // future: not missing
	}
	got := progress.CountSeasons(eps, map[int64]bool{2: true}, now, 0)
	if got[1].Imported != 1 || got[1].Total != 2 || got[2].Total != 1 || got[2].Imported != 0 {
		t.Fatalf("%+v", got)
	}
	// An episode still being broadcast (air time + runtime ahead of now) is not missing yet.
	live := []arr.Episode{{ID: 1, Season: 1, Monitored: true, AirDateUTC: now.Add(-10 * time.Minute).Format(time.RFC3339)}}
	if n := progress.CountSeasons(live, map[int64]bool{}, now, 30); len(n) != 0 {
		t.Fatalf("still airing: %+v", n)
	}
	// ... but a queued one counts even if its air date is ahead.
	if n := progress.CountSeasons(live, map[int64]bool{1: true}, now, 30); n[1].Total != 1 {
		t.Fatalf("queued: %+v", n)
	}
}

func rig(t *testing.T, eps string) (*progress.Tracker, *store.Store) {
	t.Helper()
	secret.SetKeyForTest(make([]byte, 32))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" {
			w.WriteHeader(401)
			return
		}
		switch r.URL.Path {
		case "/api/v3/history":
			if r.URL.Query().Get("downloadId") == "" { // the recently-added read
				fmt.Fprint(w, `{"records":[{"eventType":"downloadFolderImported","date":"2026-10-31T10:00:00Z","series":{"title":"24"}},{"eventType":"downloadFolderImported","date":"2026-10-31T09:00:00Z","series":{"title":"24"}},{"eventType":"grabbed","date":"2026-10-31T08:00:00Z","series":{"title":"24"}}]}`)
				return
			}
			if r.URL.Query().Get("downloadId") != "abc" {
				t.Errorf("history asked for %q", r.URL.Query().Get("downloadId"))
			}
			fmt.Fprint(w, `{"records":[{"seriesId":24,"episodeId":2},{"seriesId":24,"episodeId":2}]}`)
		case "/api/v3/series/24":
			fmt.Fprint(w, `{"title":"24","runtime":0}`)
		case "/api/v3/episode":
			fmt.Fprint(w, eps)
		case "/api/v3/wanted/missing":
			fmt.Fprint(w, `{"records":[{"title":"Pilot","seasonNumber":1,"episodeNumber":2,"airDateUtc":"2020-01-01T00:00:00Z","monitored":true,"series":{"title":"24"}},{"title":"x","monitored":false}]}`)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	env := config.Env{ConfigDir: filepath.Join(dir, "c"), DataDir: filepath.Join(dir, "d"), TickInterval: time.Second}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "tv-sonarr", Kind: "sonarr", ArrURL: srv.URL, ArrAPIKey: "k"}}
	if err := cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Open(filepath.Join(env.ConfigDir, "jobs.json"))
	if err := st.Put(job.Job{ID: "abc", Name: "24.S01E02", Category: "tv-sonarr", Source: "sonarr", State: job.Downloading}); err != nil {
		t.Fatal(err)
	}
	tr := progress.New(cfg, st, log.New(io.Discard, "", 0))
	tr.Now = func() time.Time { return now }
	return tr, st
}

func TestRefreshShowsSeasonProgressAndMissing(t *testing.T) {
	tr, _ := rig(t, `[{"id":1,"seasonNumber":1,"monitored":true,"hasFile":true},{"id":2,"seasonNumber":1,"monitored":true,"hasFile":false,"airDateUtc":"2020-01-01T00:00:00Z"}]`)
	tr.Refresh(context.Background())
	s := tr.Snapshot()
	if len(s.Errors) != 0 || len(s.Seasons) != 1 || s.Seasons[0].Show != "24" || s.Seasons[0].Imported != 1 || s.Seasons[0].Total != 2 {
		t.Fatalf("%+v", s)
	}
	if len(s.Recent) != 1 || s.Recent[0].Title != "24" || s.Recent[0].Count != 2 || s.Recent[0].Kind != "episode" {
		t.Fatalf("recent: %+v", s.Recent)
	}
	if len(s.Missing) != 1 || s.Missing[0].Title != "24" || s.Missing[0].Detail != "S01E02" || s.Missing[0].Category != "tv-sonarr" {
		t.Fatalf("missing: %+v", s.Missing)
	}
}

// A show stays listed after its job is gone, until every episode is imported, then drops off.
func TestShowStaysUntilComplete(t *testing.T) {
	tr, st := rig(t, `[{"id":1,"seasonNumber":1,"monitored":true,"hasFile":true},{"id":2,"seasonNumber":1,"monitored":true,"hasFile":false,"airDateUtc":"2020-01-01T00:00:00Z"}]`)
	tr.Refresh(context.Background())
	_, _ = st.Update("abc", func(j *job.Job) error { j.State = job.Cleaned; return nil })
	tr.Refresh(context.Background())
	if len(tr.Snapshot().Seasons) != 1 {
		t.Fatal("the show should stay while an episode is still missing")
	}
}

func TestCompleteShowDropsOff(t *testing.T) {
	tr, _ := rig(t, `[{"id":1,"seasonNumber":1,"monitored":true,"hasFile":true},{"id":2,"seasonNumber":1,"monitored":true,"hasFile":true}]`)
	tr.Refresh(context.Background())
	if n := len(tr.Snapshot().Seasons); n != 0 {
		t.Fatalf("a complete show should not be listed: %d", n)
	}
}
