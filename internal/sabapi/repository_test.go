package sabapi

import (
	"context"
	"errors"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/store"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type failedRepository struct{ cause error }

func (f failedRepository) Get(context.Context, string) (job.Job, error) { return job.Job{}, f.cause }
func (f failedRepository) List(context.Context) ([]job.Job, error)      { return nil, f.cause }
func (f failedRepository) Put(context.Context, job.Job) error           { return f.cause }
func (f failedRepository) Update(context.Context, string, func(*job.Job) error) (job.Job, error) {
	return job.Job{}, f.cause
}
func failingAPI(t *testing.T, cause error) *API {
	t.Helper()
	dir := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")})
	if err != nil {
		t.Fatal(err)
	}
	repo := failedRepository{cause}
	engine := pipeline.NewRepository(repo, cfg, nil)
	return NewRepository(repo, engine, cfg, nil)
}

func TestRepositoryFailureDoesNotEmptyQueueOrAcknowledgeDelete(t *testing.T) {
	api := failingAPI(t, errors.New("private database path"))
	for _, mode := range []string{"queue", "history", "delete"} {
		t.Run(mode, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api?value=existing", nil)
			switch mode {
			case "queue":
				api.queue(w, r)
			case "history":
				api.history(w, r)
			case "delete":
				api.deleteIDs(w, r, false)
			}
			if w.Code != 503 || strings.Contains(w.Body.String(), "private") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	api = failingAPI(t, store.ErrNotFound)
	w := httptest.NewRecorder()
	api.deleteIDs(w, httptest.NewRequest("GET", "/api?value=missing", nil), false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"status":true`) {
		t.Fatal("missing delete stopped being idempotent", w.Code, w.Body.String())
	}
}
