package pipeline

import (
	"context"
	"errors"
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
	if !e.sendToTdarr(job.Job{ID: "job", StagingDir: staging, Files: []string{"episode.mkv"}}) {
		t.Fatal("must not skip processing on storage error")
	}
	if _, err = os.Stat(src); err != nil {
		t.Fatal("staging file changed", err)
	}
	if _, err = os.Stat(filepath.Join(settings.Tdarr.InDir, "job", "episode.mkv")); !os.IsNotExist(err) {
		t.Fatal("file handed to Tdarr", err)
	}
}
