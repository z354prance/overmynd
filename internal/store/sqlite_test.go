package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

func TestSQLiteRecoveryAndFailures(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "jobs.db")
	s, err := OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if err := s.Put(ctx, job.Job{ID: "protocol-id", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "absent"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	stop := errors.New("stop")
	if _, err := s.Update(ctx, "protocol-id", func(j *job.Job) error { j.State = job.Failed; return stop }); !errors.Is(err, stop) {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := s.Update(cancelled, "protocol-id", func(j *job.Job) error { j.State = job.Failed; return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	// Trigger a real SQLite write failure after the payload update but before
	// history persistence; the entire transaction must roll back.
	if _, err := s.db.Exec(`CREATE TRIGGER reject_transition BEFORE INSERT ON transitions BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, "protocol-id", func(j *job.Job) error { j.Move(job.Downloading, "failed write", time.Now()); return nil }); err == nil {
		t.Fatal("write succeeded")
	}
	j, err := s.Get(ctx, "protocol-id")
	if err != nil || j.State != job.Received || len(j.History) != 0 {
		t.Fatalf("rollback: %+v %v", j, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_transition"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Update(ctx, "protocol-id", func(j *job.Job) error { j.Move(job.Downloading, "committed", time.Now()); return nil }); err != nil {
		t.Fatal(err)
	}
	intent := ImportIntent{ID: "file-1", JobID: j.ID, Source: "/fixture/input.mkv", Destination: "/fixture/output.mkv", State: "copying"}
	if err := s.SaveIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	invalid := intent
	invalid.JobID = "absent"
	invalid.ID = "orphan"
	if err := s.SaveIntent(ctx, invalid); err == nil {
		t.Fatal("foreign key not enforced")
	}
	invalid = intent
	invalid.State = "cleaned"
	if err := s.SaveIntent(ctx, invalid); err == nil {
		t.Fatal("cleanup accepted as import success")
	}
	invalid = intent
	invalid.Destination = "/different"
	if err := s.SaveIntent(ctx, invalid); err == nil {
		t.Fatal("identity replaced")
	}
	s.Close()
	s, err = OpenSQLite(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, j.ID)
	if err != nil || got.State != job.Downloading {
		t.Fatalf("restart: %+v %v", got, err)
	}
	intents, err := s.Intents(ctx, j.ID)
	if err != nil || len(intents) != 1 || intents[0].State != "copying" {
		t.Fatalf("intents: %+v %v", intents, err)
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM transitions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("history: %d %v", count, err)
	}
	s.Close()
	if _, err := s.Get(ctx, j.ID); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("database failure hidden: %v", err)
	}
	if _, err := s.List(ctx); err == nil {
		t.Fatal("list failure hidden")
	}
}

func TestSQLiteCompetingWritersAndHistory(t *testing.T) {
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
	if err := a.Put(ctx, job.Job{ID: "a", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := a
			if i%2 == 0 {
				s = b
			}
			_, err := s.Update(ctx, "a", func(j *job.Job) error { j.Attempts++; return nil })
			if err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, err := a.Get(ctx, "a")
	if err != nil || got.Attempts != 20 {
		t.Fatalf("lost update: %+v %v", got, err)
	}
	// Move bounds payload history at 200; the transition journal must retain all.
	for i := 0; i < 205; i++ {
		_, err := a.Update(ctx, "a", func(j *job.Job) error {
			to := job.Downloading
			if j.State == to {
				to = job.Pulling
			}
			j.Move(to, "step", time.Now())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := a.db.QueryRow("SELECT count(*) FROM transitions").Scan(&count); err != nil || count != 205 {
		t.Fatalf("history lost: %d %v", count, err)
	}
}

func TestSQLiteDiskFullAndCommitCancellation(t *testing.T) {
	ctx := context.Background()
	s, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Put(ctx, job.Job{ID: "a", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	_, err = s.Update(cancelled, "a", func(j *job.Job) error { j.State = job.Failed; cancel(); return nil })
	if err == nil {
		t.Fatal("cancelled transaction published")
	}
	// Cap database growth at its current allocation to produce SQLITE_FULL,
	// without filling the host disk or depending on Unix ownership privileges.
	var pages int
	if err := s.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
		t.Fatal(err)
	}
	var limit int
	if err := s.db.QueryRow(fmt.Sprintf("PRAGMA max_page_count=%d", pages)).Scan(&limit); err != nil {
		t.Fatal(err)
	}
	_, err = s.Update(ctx, "a", func(j *job.Job) error {
		j.Name = strings.Repeat("x", 1024*1024)
		j.Move(job.Failed, "full", time.Now())
		return nil
	})
	if err == nil {
		t.Fatal("expected disk full")
	}
	var sqliteError interface{ Code() int }
	if !errors.As(err, &sqliteError) || sqliteError.Code() != 13 {
		t.Fatalf("expected SQLITE_FULL, got %v", err)
	}
	got, err := s.Get(ctx, "a")
	if err != nil || got.State != job.Received || got.Name != "" {
		t.Fatalf("uncommitted state: %+v %v", got, err)
	}
	history, err := s.Transitions(ctx, "a")
	if err != nil || len(history) != 0 {
		t.Fatalf("uncommitted history: %+v %v", history, err)
	}
}
