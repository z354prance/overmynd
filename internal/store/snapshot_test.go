package store

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

func TestSnapshotImportRestart(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	s, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	j := job.Job{ID: "original-protocol-id", State: job.Submitted, RemoteID: "existing-download", Files: []string{"episode.mkv"}}
	j.Move(job.Downloading, "fixture", time.Now().UTC())
	data, err := json.Marshal([]job.Job{j})
	if err != nil {
		t.Fatal(err)
	}
	if imported, err := s.ImportSnapshot(ctx, data); err != nil || !imported {
		t.Fatalf("import: %v %v", imported, err)
	}
	s.Close()
	s, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if imported, err := s.ImportSnapshot(ctx, data); err != nil || imported {
		t.Fatalf("repeat: %v %v", imported, err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil || !reflect.DeepEqual(got, j) {
		t.Fatalf("restored: %+v %v", got, err)
	}
	history, err := s.Transitions(ctx, j.ID)
	if err != nil || !reflect.DeepEqual(history, j.History) {
		t.Fatalf("history: %+v %v", history, err)
	}
	if _, err := s.ImportSnapshot(ctx, []byte("[]")); err == nil {
		t.Fatal("different snapshot accepted")
	}
}

func TestSnapshotRejectsInvalidAndRollsBack(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, data := range []string{
		`null`, `{}`, `[null]`, `[{"id":"","state":"received"}]`,
		`[{"id":"a","state":"unknown"}]`,
		`[{"id":"a","state":"received"},{"id":"a","state":"received"}]`,
		`[{"id":"a","state":"received","history":[{"from":"received","to":"submitted"}]}]`,
	} {
		if _, err := s.ImportSnapshot(ctx, []byte(data)); err == nil {
			t.Fatalf("accepted %s", data)
		}
	}
	data := []byte(`[{"id":"a","state":"received"},{"id":"b","state":"submitted","history":[{"from":"received","to":"submitted"}]}]`)
	if _, err := s.db.Exec(`CREATE TRIGGER reject_snapshot BEFORE INSERT ON transitions BEGIN SELECT RAISE(ABORT, 'injected'); END`); err != nil {
		t.Fatal(err)
	}
	if imported, err := s.ImportSnapshot(ctx, data); err == nil || imported {
		t.Fatalf("failed import: %v %v", imported, err)
	}
	jobs, err := s.List(ctx)
	if err != nil || len(jobs) != 0 {
		t.Fatalf("partial jobs: %+v %v", jobs, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_snapshot"); err != nil {
		t.Fatal(err)
	}
	if imported, err := s.ImportSnapshot(ctx, data); err != nil || !imported {
		t.Fatalf("retry: %v %v", imported, err)
	}
}

func TestSnapshotRefusesExistingJobsAndCancellation(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.ImportSnapshot(cancelled, []byte("[]")); err == nil {
		t.Fatal("cancelled import succeeded")
	}
	if err := s.Put(ctx, job.Job{ID: "existing", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ImportSnapshot(ctx, []byte("[]")); err == nil {
		t.Fatal("existing jobs accepted")
	}
}

func TestSnapshotConcurrentImport(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	a, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	data := []byte(`[{"id":"same-id","state":"submitted","history":[{"from":"received","to":"submitted"}]}]`)
	var wg sync.WaitGroup
	results := make(chan bool, 2)
	for _, s := range []*SQLite{a, b} {
		wg.Add(1)
		go func(s *SQLite) {
			defer wg.Done()
			imported, err := s.ImportSnapshot(ctx, data)
			if err != nil {
				t.Error(err)
			}
			results <- imported
		}(s)
	}
	wg.Wait()
	close(results)
	count := 0
	for imported := range results {
		if imported {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("committed imports: %d", count)
	}
	history, err := a.Transitions(ctx, "same-id")
	if err != nil || len(history) != 1 {
		t.Fatalf("duplicated history: %+v %v", history, err)
	}
}
