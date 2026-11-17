package progress

import (
	"context"
	"errors"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/store"
	"path/filepath"
	"testing"
	"time"
)

type unavailableJobs struct{ store.Repository }

func (unavailableJobs) List(context.Context) ([]job.Job, error) {
	return nil, errors.New("private SQL error")
}
func TestFailedReadPreservesLastProgressSnapshot(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")})
	if err != nil {
		t.Fatal(err)
	}
	tracker := NewRepository(cfg, unavailableJobs{}, nil)
	previous := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker.snap = Snapshot{UpdatedAt: previous, Seasons: []Season{{Show: "Existing show", Imported: 2, Total: 3}}, Recent: []Recent{{Title: "Existing addition"}}}
	tracker.Refresh(context.Background())
	got := tracker.Snapshot()
	if !got.UpdatedAt.Equal(previous) || len(got.Seasons) != 1 || len(got.Recent) != 1 || len(got.Errors) != 1 || got.Errors[0] != "job storage is unavailable" {
		t.Fatalf("last good snapshot lost: %+v", got)
	}
}
