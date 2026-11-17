package pipeline

import (
	"context"
	"errors"
	"github.com/z354prance/overmynd/internal/backend"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/store"
	"os"
	"path/filepath"
	"testing"
)

type capacityFailure struct {
	store.Repository
	cause error
}

func (r capacityFailure) List(context.Context) ([]job.Job, error) { return nil, r.cause }
func TestTdarrReadFailureLeavesStagingAlone(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(root, "config"), DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	settings := cfg.Get()
	settings.Tdarr.InDir = filepath.Join(root, "tdarr-in")
	if err = cfg.Set(settings); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(root, "staging")
	if err = os.MkdirAll(staging, 0700); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(staging, "episode.mkv")
	if err = os.WriteFile(src, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	e := NewRepository(capacityFailure{cause: errors.New("unavailable")}, cfg, nil)
	if !e.sendToTdarr(context.Background(), job.Job{ID: "job", StagingDir: staging, Files: []string{"episode.mkv"}}) {
		t.Fatal("must not skip processing on storage error")
	}
	if _, err = os.Stat(src); err != nil {
		t.Fatal("staging file changed", err)
	}
	if _, err = os.Stat(filepath.Join(settings.Tdarr.InDir, "job", "episode.mkv")); !os.IsNotExist(err) {
		t.Fatal("file handed to Tdarr", err)
	}
}

type rejectedUpdate struct {
	store.Repository
	writes int
}

func (r *rejectedUpdate) Update(context.Context, string, func(*job.Job) error) (job.Job, error) {
	r.writes++
	return job.Job{}, errors.New("commit failed")
}

type statusOnly struct{ backend.Backend }

func (statusOnly) Status(context.Context, string) (backend.Item, error) {
	return backend.Item{Status: backend.Complete}, nil
}
func TestFailedRetryResetDoesNotUseAnEmptyJob(t *testing.T) {
	root := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(root, "config"), DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	repo := &rejectedUpdate{}
	e := NewRepository(repo, cfg, nil)
	e.Override = statusOnly{}
	e.doDownload(context.Background(), job.Job{ID: "keep-id", State: job.Downloading, Attempts: 1})
	if repo.writes != 1 {
		t.Fatal("continued after failed retry commit", repo.writes)
	}
}
