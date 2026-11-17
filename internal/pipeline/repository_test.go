package pipeline_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/store"
)

type brokenRepository struct {
	cause         error
	reads, writes int
}

func (b *brokenRepository) Get(context.Context, string) (job.Job, error) {
	b.reads++
	return job.Job{}, b.cause
}
func (b *brokenRepository) List(context.Context) ([]job.Job, error) { b.reads++; return nil, b.cause }
func (b *brokenRepository) Put(context.Context, job.Job) error      { b.writes++; return nil }
func (b *brokenRepository) Update(context.Context, string, func(*job.Job) error) (job.Job, error) {
	b.writes++
	return job.Job{}, nil
}

func brokenEngine(t *testing.T) (*pipeline.Engine, *brokenRepository) {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")})
	if err != nil {
		t.Fatal(err)
	}
	repo := &brokenRepository{cause: errors.New("storage read failure")}
	return pipeline.NewRepository(repo, cfg, nil), repo
}

func TestIntakeDoesNotTreatRepositoryFailuresAsMissing(t *testing.T) {
	for _, kind := range []string{"torrent", "usenet", "reencode"} {
		t.Run(kind, func(t *testing.T) {
			e, repo := brokenEngine(t)
			var err error
			switch kind {
			case "torrent":
				_, err = e.Submit(pipeline.SubmitRequest{Magnet: "magnet:?xt=urn:btih:1111111111111111111111111111111111111111&dn=Example"})
			case "usenet":
				_, err = e.SubmitNZB(pipeline.SubmitNZBRequest{Name: "Example", NZB: []byte(`<nzb></nzb>`)})
			case "reencode":
				dir := t.TempDir()
				src := filepath.Join(dir, "Example.mkv")
				if writeErr := os.WriteFile(src, []byte("fixture"), 0600); writeErr != nil {
					t.Fatal(writeErr)
				}
				e.LibraryRoots = []string{dir}
				_, err = e.SubmitReencode(src)
			}
			if !errors.Is(err, repo.cause) {
				t.Fatalf("read failure lost: %v", err)
			}
			if repo.reads != 1 || repo.writes != 0 {
				t.Fatalf("unexpected work: %+v", repo)
			}
		})
	}
}

func TestReadFailuresStopJobLoopAndCleanup(t *testing.T) {
	e, repo := brokenEngine(t)
	e.Tick(context.Background())
	if repo.reads != 1 || repo.writes != 0 {
		t.Fatalf("loop performed work: %+v", repo)
	}
	if err := e.CleanupNow("existing"); !errors.Is(err, repo.cause) {
		t.Fatalf("cleanup lost cause: %v", err)
	}
	if _, err := e.System(); !errors.Is(err, repo.cause) {
		t.Fatalf("system returned false counts: %v", err)
	}
	if repo.writes != 0 {
		t.Fatalf("read failure caused updates: %+v", repo)
	}
}

func TestSQLiteClosedReadStopsIntake(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")})
	if err != nil {
		t.Fatal(err)
	}
	repo, err := store.OpenSQLite(context.Background(), filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Close(); err != nil {
		t.Fatal(err)
	}
	e := pipeline.NewRepository(repo, cfg, nil)
	if _, err = e.SubmitNZB(pipeline.SubmitNZBRequest{Name: "Example", NZB: []byte(`<nzb></nzb>`)}); err == nil || errors.Is(err, store.ErrNotFound) {
		t.Fatalf("closed SQLite was treated as missing: %v", err)
	}
}

func TestCancelledIntakeDoesNotTouchStorage(t *testing.T) {
	e, repo := brokenEngine(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := e.SubmitContext(ctx, pipeline.SubmitRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.SubmitNZBContext(ctx, pipeline.SubmitNZBRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := e.SubmitReencodeContext(ctx, "unused"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if repo.reads != 0 || repo.writes != 0 {
		t.Fatalf("cancelled intake touched storage: %+v", repo)
	}
}
