package qbitapi

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

func TestRepositoryFailuresAreNotEmptyOrMissing(t *testing.T) {
	api := failingAPI(t, errors.New("private database path"))
	for _, handler := range []string{"info", "properties", "files", "delete"} {
		t.Run(handler, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/unused?hash=existing&hashes=existing", nil)
			switch handler {
			case "info":
				api.info(w, r)
			case "properties":
				api.properties(w, r)
			case "files":
				api.files(w, r)
			case "delete":
				api.delete(w, r)
			}
			if w.Code != 503 || strings.Contains(w.Body.String(), "private") {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	api = failingAPI(t, store.ErrNotFound)
	w := httptest.NewRecorder()
	api.properties(w, httptest.NewRequest("GET", "/?hash=missing", nil))
	if w.Code != 404 {
		t.Fatal("missing was not distinguished", w.Code)
	}
}
