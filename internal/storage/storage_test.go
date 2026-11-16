package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCategoriesAndMissingAreNotZero(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Movies", "Episodes", "Anime", "Books", "Unrelated"} {
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(root, "Movies", "movie"), []byte("12345"), 0600)
	os.WriteFile(filepath.Join(root, "Episodes", "episode"), []byte("123"), 0600)
	os.WriteFile(filepath.Join(root, "Unrelated", "private"), []byte("123456789"), 0600)
	result := scan(root)
	if len(result.Categories) != 5 || *result.Categories[0].Bytes != 5 || *result.Categories[1].Bytes != 3 {
		t.Fatal(result)
	}
	if result.Categories[3].Bytes != nil || *result.Categories[4].Bytes != 0 {
		t.Fatal("missing and empty confused")
	}
}
func TestReadingNeverScansAndDisabledStaysOff(t *testing.T) {
	if result := New("").Snapshot(); result.Enabled || result.Scanning {
		t.Fatal(result)
	}
	m := New(t.TempDir())
	if first := m.Snapshot(); first.Scanning || !first.UpdatedAt.IsZero() {
		t.Fatal("a read started a scan", first)
	}
}
func TestScanIsSavedAndLoadedAfterARestart(t *testing.T) {
	root, file := t.TempDir(), filepath.Join(t.TempDir(), "scan.json")
	os.Mkdir(filepath.Join(root, "Movies"), 0700)
	os.WriteFile(filepath.Join(root, "Movies", "movie"), []byte("12345"), 0600)
	m := New(root)
	m.Persist(file)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, func() time.Duration { return time.Hour }); close(done) }()
	deadline := time.Now().Add(3 * time.Second)
	for m.Snapshot().UpdatedAt.IsZero() {
		if time.Now().After(deadline) {
			t.Fatal("no scan ran")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	scanned := m.Snapshot()
	if scanned.Scanning || *scanned.Categories[0].Bytes != 5 {
		t.Fatal(scanned)
	}
	again := New(root)
	again.Persist(file)
	loaded := again.Snapshot()
	if !loaded.UpdatedAt.Equal(scanned.UpdatedAt) || !loaded.Enabled || loaded.Scanning || *loaded.Categories[0].Bytes != 5 {
		t.Fatal("saved scan not loaded", loaded)
	}
}
func TestFreshSavedScanIsNotRepeatedAtStartup(t *testing.T) {
	root, file := t.TempDir(), filepath.Join(t.TempDir(), "scan.json")
	m := New(root)
	m.Persist(file)
	m.scanNow()
	saved := m.Snapshot().UpdatedAt
	again := New(root)
	again.Persist(file)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	again.Run(ctx, func() time.Duration { return time.Hour })
	if !again.Snapshot().UpdatedAt.Equal(saved) {
		t.Fatal("scanned again although the saved scan was fresh")
	}
}
func TestSymlinksAreNotFollowed(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "private"), []byte("secret"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip(err)
	}
	size, err := directorySize(root)
	if err != nil || size != 0 {
		t.Fatal(size, err)
	}
	if _, err = directorySize(filepath.Join(root, "link")); err == nil {
		t.Fatal("followed root link")
	}
}
