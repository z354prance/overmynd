package web

import (
	"context"
	"errors"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

type readFailure struct {
	store.Repository
	cause error
}

func (f readFailure) Get(context.Context, string) (job.Job, error) { return job.Job{}, f.cause }
func (f readFailure) List(context.Context) ([]job.Job, error)      { return nil, f.cause }
func TestStorageFailuresDoNotClearDashboard(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.Load(config.Env{ConfigDir: filepath.Join(dir, "config"), DataDir: filepath.Join(dir, "data")})
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: readFailure{cause: errors.New("private path or SQL error")}, Cfg: cfg}
	for name, handler := range map[string]http.HandlerFunc{"public": s.publicJobs, "admin": s.jobs, "detail": s.jobDetail, "events": s.events, "activity": s.v1Activity} {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			r.SetPathValue("id", "existing")
			handler(w, r)
			if w.Code != 503 || strings.Contains(w.Body.String(), "private") || strings.TrimSpace(w.Body.String()) == "[]" {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
	s.Store = readFailure{cause: store.ErrNotFound}
	w := httptest.NewRecorder()
	s.jobDetail(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 404 {
		t.Fatal("missing job must stay distinct", w.Code)
	}
}
