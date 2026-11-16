package rss_test

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"context"

	"github.com/z354prance/undermynd/internal/config"
	"github.com/z354prance/undermynd/internal/job"
	"github.com/z354prance/undermynd/internal/pipeline"
	"github.com/z354prance/undermynd/internal/rss"
	"github.com/z354prance/undermynd/internal/secret"
	"github.com/z354prance/undermynd/internal/store"
)

func feed(items ...string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><rss><channel>`)
	for _, i := range items {
		b.WriteString(i)
	}
	b.WriteString(`</channel></rss>`)
	return b.String()
}

func magnetItem(title, hash string) string {
	return fmt.Sprintf(`<item><title>%s</title><guid>%s</guid><link>magnet:?xt=urn:btih:%s&amp;dn=x</link></item>`, title, hash, hash)
}

func TestRSS(t *testing.T) {
	secret.SetKeyForTest(make([]byte, 32))
	dir := t.TempDir()
	env := config.Env{ConfigDir: filepath.Join(dir, "c"), DataDir: filepath.Join(dir, "d"), TickInterval: time.Second}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	body := feed(magnetItem("Old.Show.S01E01.1080p", "1111111111111111111111111111111111111111"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		io.WriteString(w, body)
	}))
	defer srv.Close()
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "tv-sonarr", Kind: "sonarr"}}
	s.Feeds = []config.Feed{{Name: "tracker", URL: srv.URL, IntervalMinutes: 1, Enabled: true}}
	s.Rules = []config.Rule{{Name: "1080p shows", Feed: "TRACKER ", MustContain: `S\d+E\d+.*1080p`, MustNotContain: `cam|hdts`, Category: "tv-sonarr", Enabled: true}}
	if err := cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	st, _ := store.Open(filepath.Join(env.ConfigDir, "jobs.json"))
	eng := pipeline.New(st, cfg, log.New(io.Discard, "", 0))
	p := rss.New(cfg, eng, log.New(io.Discard, "", 0))
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	p.Now = func() time.Time { return now }

	p.Tick(context.Background())
	if len(st.List()) != 0 {
		t.Fatal("the first poll only records existing items")
	}
	mu.Lock()
	body = feed(
		magnetItem("New.Show.S02E03.1080p.WEB", "2222222222222222222222222222222222222222"),
		magnetItem("Other.Show.S01E01.720p", "3333333333333333333333333333333333333333"),
		magnetItem("Cam.Show.S01E01.1080p.CAM", "4444444444444444444444444444444444444444"),
		magnetItem("Old.Show.S01E01.1080p", "1111111111111111111111111111111111111111"),
	)
	mu.Unlock()
	p.Tick(context.Background())
	if len(st.List()) != 0 {
		t.Fatal("must not poll again before the interval")
	}
	now = now.Add(2 * time.Minute)
	p.Tick(context.Background())
	jobs := st.List()
	if len(jobs) != 1 || jobs[0].Name != "New.Show.S02E03.1080p.WEB" && jobs[0].ID != "2222222222222222222222222222222222222222" {
		t.Fatalf("expected exactly the matching new item, got %+v", jobs)
	}
	if jobs[0].Source != "rss" || jobs[0].State != job.Received {
		t.Fatalf("rss job should be marked as rss: %+v", jobs[0])
	}
	got := map[string]string{}
	for _, f := range p.Status() {
		for _, it := range f.Items {
			got[it.Title] = it.Outcome
		}
	}
	if !strings.HasPrefix(got["New.Show.S02E03.1080p.WEB"], "started by rule") ||
		got["Other.Show.S01E01.720p"] != "no rule matched" ||
		got["Cam.Show.S01E01.1080p.CAM"] != "no rule matched" {
		t.Fatalf("status should say what happened to each item: %+v", got)
	}
	// A skipped item can be started by hand, only as the list was last shown.
	idx, title := -1, "Other.Show.S01E01.720p"
	for _, f := range p.Status() {
		for n, it := range f.Items {
			if it.Title == title {
				idx = n
			}
		}
	}
	if _, err := p.Download(context.Background(), "tracker", idx, "wrong title", "tv-sonarr"); err == nil {
		t.Fatal("a changed list must be refused")
	}
	if _, err := p.Download(context.Background(), "tracker", idx, title, "nope"); err == nil {
		t.Fatal("an unknown category must be refused")
	}
	if _, err := p.Download(context.Background(), "tracker", idx, title, "tv-sonarr"); err != nil {
		t.Fatal(err)
	}
	if len(st.List()) != 2 {
		t.Fatalf("the manual download should create a job: %+v", st.List())
	}
	now = now.Add(2 * time.Minute)
	p.Tick(context.Background())
	if len(st.List()) != 2 {
		t.Fatal("an item is only ever acted on once")
	}
}
