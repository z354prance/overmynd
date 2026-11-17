package store

import (
	"context"
	"errors"
	"github.com/z354prance/overmynd/internal/job"
	"path/filepath"
	"testing"
)

func TestLegacyAdapterPreservesMissingAndCancellation(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	repo := FromLegacy(source)
	ctx := context.Background()
	if _, err = repo.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = repo.Put(cancelled, job.Job{ID: "new"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = repo.Update(cancelled, "new", func(*job.Job) error { t.Fatal("cancelled callback executed"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = repo.Get(cancelled, "new"); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err = repo.List(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, ok := source.Get("new"); ok {
		t.Fatal("cancelled write created a job")
	}
	if err = repo.Put(ctx, job.Job{ID: "new", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	if _, err = repo.Get(ctx, "new"); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyAdapterCancelledUpdateDoesNotPublish(t *testing.T) {
	source, err := Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = source.Put(job.Job{ID: "keep", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	_, err = FromLegacy(source).Update(ctx, "keep", func(j *job.Job) error { j.State = job.Failed; cancel(); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if j, ok := source.Get("keep"); !ok || j.State != job.Received {
		t.Fatal("cancelled update was published", j)
	}
}
