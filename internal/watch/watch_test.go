package watch_test

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z354prance/undermynd/internal/config"
	"github.com/z354prance/undermynd/internal/job"
	"github.com/z354prance/undermynd/internal/pipeline"
	"github.com/z354prance/undermynd/internal/watch"
)

type fake struct {
	mu   sync.Mutex
	torr []pipeline.SubmitRequest
	nzbs []pipeline.SubmitNZBRequest
	fail bool
}

func (f *fake) Submit(r pipeline.SubmitRequest) (job.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		return job.Job{}, os.ErrInvalid
	}
	f.torr = append(f.torr, r)
	return job.Job{ID: "abcdef0123456789"}, nil
}
func (f *fake) SubmitNZB(r pipeline.SubmitNZBRequest) (job.Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nzbs = append(f.nzbs, r)
	return job.Job{ID: "nzo"}, nil
}

func setup(t *testing.T, cat string) (*watch.Poller, *fake, string) {
	dir := t.TempDir()
	env := config.Env{ConfigDir: filepath.Join(dir, "c"), DataDir: filepath.Join(dir, "d")}
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	drop := filepath.Join(dir, "drop")
	_ = os.MkdirAll(drop, 0o755)
	s := cfg.Get()
	s.Categories = []config.Category{{Name: "tv", Kind: "sonarr"}}
	s.Rules = []config.Rule{{Name: "r", Category: "tv", Enabled: true}}
	s.Watches = []config.Watch{{Dir: drop, Category: cat, Enabled: true}}
	_ = cfg.Set(s)
	f := &fake{}
	p := watch.New(cfg, f, log.New(io.Discard, "", 0))
	p.Now = func() time.Time { return time.Now().Add(time.Minute) } // every file counts as settled
	return p, f, drop
}

func write(t *testing.T, dir, name, body string) {
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDroppedFilesAreSubmittedOnceAndMovedAside(t *testing.T) {
	p, f, drop := setup(t, "")
	write(t, drop, "a.torrent", "d4:infod4:name1:xee")
	write(t, drop, "b.nzb", "<nzb></nzb>")
	write(t, drop, "c.magnet", "magnet:?xt=urn:btih:0123456789abcdef0123456789abcdef01234567&dn=x\n")
	write(t, drop, "notes.txt", "ignore me")
	p.Scan()
	if len(f.torr) != 2 || len(f.nzbs) != 1 {
		t.Fatalf("expected 2 torrents (file + magnet) and 1 nzb, got %d and %d", len(f.torr), len(f.nzbs))
	}
	for _, r := range f.torr {
		if r.Source != "watch" || r.Category != "tv" {
			t.Fatalf("the category defaults to the first RSS rule and the source is watch: %+v", r)
		}
	}
	if !strings.HasPrefix(f.torr[len(f.torr)-1].Magnet+f.torr[0].Magnet, "magnet:") {
		t.Fatal("the magnet link must be passed through")
	}
	for _, n := range []string{"a.torrent", "b.nzb", "c.magnet"} {
		if _, err := os.Stat(filepath.Join(drop, watch.Submitted, n)); err != nil {
			t.Fatalf("%s should be moved to the submitted folder: %v", n, err)
		}
	}
	if _, err := os.Stat(filepath.Join(drop, "notes.txt")); err != nil {
		t.Fatal("other files must be left alone")
	}
	p.Scan()
	if len(f.torr) != 2 || len(f.nzbs) != 1 {
		t.Fatal("a handled file must not be submitted again")
	}
}

func TestBadFilesAreSetAsideAndAFreshFileIsLeftToSettle(t *testing.T) {
	p, f, drop := setup(t, "tv")
	f.fail = true
	write(t, drop, "bad.torrent", "garbage")
	p.Scan()
	if _, err := os.Stat(filepath.Join(drop, watch.Rejected, "bad.torrent")); err != nil {
		t.Fatalf("an unreadable file should be set aside: %v", err)
	}
	p.Now = time.Now // now files count as just written
	write(t, drop, "new.torrent", "d4:infod4:name1:xee")
	f.fail = false
	p.Scan()
	if len(f.torr) != 0 {
		t.Fatal("a file still being written must wait")
	}
}
