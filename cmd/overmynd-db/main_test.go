package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/z354prance/overmynd/internal/store"
)

func TestImportBackupRestoreCommand(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	capture := filepath.Join(dir, "captured.json")
	data := []byte(`[{"id":"keep-id","state":"submitted","remoteId":"keep-remote","history":[{"from":"received","to":"submitted"}]}]`)
	if err := os.WriteFile(capture, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db := filepath.Join(dir, "jobs.db")
	args := []string{"import", "-database", db, "-snapshot", capture}
	for _, expected := range []bool{true, false} {
		var out bytes.Buffer
		if err := run(ctx, args, &out); err != nil {
			t.Fatal(err)
		}
		var result struct {
			Imported bool
			Jobs     int
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil || result.Imported != expected || result.Jobs != 1 {
			t.Fatalf("import result %q: %v", out.String(), err)
		}
	}
	backup := filepath.Join(dir, "restore.db")
	var out bytes.Buffer
	if err := run(ctx, []string{"backup", "-database", db, "-output", backup}, &out); err != nil {
		t.Fatal(err)
	}
	restored, err := store.OpenSQLite(ctx, backup)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	j, err := restored.Get(ctx, "keep-id")
	if err != nil || j.RemoteID != "keep-remote" {
		t.Fatalf("restored job: %+v %v", j, err)
	}
	if imported, err := restored.ImportSnapshot(ctx, data); err != nil || imported {
		t.Fatalf("restored marker: %v %v", imported, err)
	}
	original, err := os.ReadFile(capture)
	if err != nil || !bytes.Equal(original, data) {
		t.Fatal("source capture changed")
	}
}

func TestInvalidCommandsDoNotCreateDatabase(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "absent.db")
	capture := filepath.Join(dir, "missing.json")
	cases := [][]string{
		nil,
		{"unknown"},
		{"import", "-database", db},
		{"import", "-database", db, "-snapshot", capture},
		{"import", "-database", db, "-snapshot", capture, "extra"},
		{"backup", "-database", db, "-output", filepath.Join(dir, "backup.db")},
	}
	for _, args := range cases {
		var out bytes.Buffer
		if err := run(context.Background(), args, &out); err == nil {
			t.Fatalf("accepted %v", args)
		}
		if out.Len() != 0 {
			t.Fatalf("reported success after failure: %q", out.String())
		}
		if _, err := os.Stat(db); !os.IsNotExist(err) {
			t.Fatalf("created absent database: %v", err)
		}
	}
}

func TestBackupRejectsNonSQLiteAndExistingOutput(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	invalid := filepath.Join(dir, "invalid")
	if err := os.WriteFile(invalid, []byte("not a SQLite database"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(ctx, []string{"backup", "-database", invalid, "-output", filepath.Join(dir, "output")}, &out); err == nil {
		t.Fatal("accepted non-SQLite source")
	}
	s, err := store.OpenSQLite(ctx, filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	if err := run(ctx, []string{"backup", "-database", filepath.Join(dir, "jobs.db"), "-output", invalid}, &out); err == nil {
		t.Fatal("overwrote existing output")
	}
	data, err := os.ReadFile(invalid)
	if err != nil || string(data) != "not a SQLite database" {
		t.Fatal("existing output changed")
	}
}
