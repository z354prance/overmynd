package store

import (
	"context"
	"github.com/z354prance/overmynd/internal/job"
	"path/filepath"
	"testing"
)

func TestSQLitePathWithURICharacters(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs #1 + 100%.db")
	ctx := context.Background()
	s, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Put(ctx, job.Job{ID: "existing", State: job.Received}); err != nil {
		s.Close()
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if j, err := s.Get(ctx, "existing"); err != nil || j.ID != "existing" {
		t.Fatal(j, err)
	}
}
