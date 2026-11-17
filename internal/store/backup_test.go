package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

func TestSQLiteBackupRestore(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	source, err := OpenSQLite(ctx, filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	snapshot := []byte(`[{"id":"legacy-id","state":"received","remoteId":"remote-keep","pastRemoteIds":["earlier"]}]`)
	if _, err := source.ImportSnapshot(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := source.Update(ctx, "legacy-id", func(j *job.Job) error {
		j.Move(job.Downloading, "committed before backup", time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	intent := ImportIntent{ID: "file-1", JobID: "legacy-id", Source: "/fixture/a", Destination: "/fixture/b", State: "copying"}
	if err := source.SaveIntent(ctx, intent); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(dir, "backup.db")
	// Keep source open: this exercises the supported live backup operation.
	if err := source.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err := source.Put(ctx, job.Job{ID: "after-backup", State: job.Received}); err != nil {
		t.Fatal(err)
	}
	restored, err := OpenSQLite(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	jobs, err := restored.List(ctx)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("snapshot count: %d %v", len(jobs), err)
	}
	got := jobs[0]
	if got.ID != "legacy-id" || got.State != job.Downloading || got.RemoteID != "remote-keep" || len(got.PastRemoteIDs) != 1 || got.PastRemoteIDs[0] != "earlier" {
		t.Fatalf("restored payload: %+v", got)
	}
	history, err := restored.Transitions(ctx, got.ID)
	if err != nil || len(history) != 1 || history[0].To != job.Downloading {
		t.Fatalf("restored journal: %+v %v", history, err)
	}
	intents, err := restored.Intents(ctx, got.ID)
	if err != nil || len(intents) != 1 || intents[0] != intent {
		t.Fatalf("restored intents: %+v %v", intents, err)
	}
	if imported, err := restored.ImportSnapshot(ctx, snapshot); err != nil || imported {
		t.Fatalf("migration marker not restored: %v %v", imported, err)
	}
	var result string
	if err := restored.db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&result); err != nil || result != "ok" {
		t.Fatalf("restored integrity: %s %v", result, err)
	}
}

func TestSQLiteBackupFailuresPreserveFiles(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := OpenSQLite(ctx, filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	existing := filepath.Join(dir, "existing")
	if err := os.WriteFile(existing, []byte("preserve"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.Backup(ctx, existing); err == nil {
		t.Fatal("existing destination accepted")
	}
	data, err := os.ReadFile(existing)
	if err != nil || string(data) != "preserve" {
		t.Fatalf("existing file changed: %q %v", data, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	failed := filepath.Join(dir, "cancelled.db")
	if err := s.Backup(cancelled, failed); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(failed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cancelled backup left output: %v", err)
	}
	s.Close()
	failed = filepath.Join(dir, "closed.db")
	if err := s.Backup(ctx, failed); err == nil {
		t.Fatal("closed repository accepted")
	}
	if _, err := os.Stat(failed); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed backup left output: %v", err)
	}
}
