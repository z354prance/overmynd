package storage

import (
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
func TestSnapshotCachesAndDisabledDoesNotScan(t *testing.T) {
	if result := New("").Snapshot(); result.Enabled || result.Scanning {
		t.Fatal(result)
	}
	m := New(t.TempDir())
	m.Snapshot()
	deadline := time.Now().Add(3 * time.Second)
	for m.Snapshot().Scanning {
		if time.Now().After(deadline) {
			t.Fatal("scan stuck")
		}
		time.Sleep(time.Millisecond)
	}
	first := m.Snapshot()
	if !first.UpdatedAt.Equal(m.Snapshot().UpdatedAt) {
		t.Fatal("read rescanned")
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
