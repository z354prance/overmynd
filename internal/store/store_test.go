package store

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

func TestFailedSaveDoesNotPublish(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	original := job.Job{ID: "original", State: job.Received, PastRemoteIDs: []string{"old"}}
	if err := s.Put(original); err != nil {
		t.Fatal(err)
	}
	// A directory at the temporary-file path deterministically rejects writes,
	// including when the test runs as root. This simulates an I/O failure.
	if err := os.Mkdir(path+".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(job.Job{ID: "new"}); err == nil {
		t.Fatal("expected failed Put")
	}
	if _, ok := s.Get("new"); ok {
		t.Fatal("failed insert became visible")
	}
	if _, err := s.Update("original", func(j *job.Job) error {
		j.Move(job.Downloading, "test", time.Now())
		j.PastRemoteIDs[0] = "changed"
		return nil
	}); err == nil {
		t.Fatal("expected failed Update")
	}
	got, _ := s.Get("original")
	if got.State != job.Received || len(got.History) != 0 || got.PastRemoteIDs[0] != "old" {
		t.Fatalf("failed update leaked: %+v", got)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	disk, _ := reopened.Get("original")
	if disk.State != got.State || disk.PastRemoteIDs[0] != "old" {
		t.Fatal("memory and restart disagree")
	}
}

func TestCallbackRollbackAndStableID(t *testing.T) {
	s, _ := Open(filepath.Join(t.TempDir(), "jobs.json"))
	if err := s.Put(job.Job{ID: "a", PastRemoteIDs: []string{"old"}}); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("cancel update")
	_, err := s.Update("a", func(j *job.Job) error {
		j.PastRemoteIDs[0] = "changed"
		return stop
	})
	if !errors.Is(err, stop) {
		t.Fatal(err)
	}
	_, err = s.Update("a", func(j *job.Job) error { j.ID = "b"; return nil })
	if err == nil {
		t.Fatal("ID change accepted")
	}
	got, _ := s.Get("a")
	if got.ID != "a" || got.PastRemoteIDs[0] != "old" {
		t.Fatalf("rollback leaked: %+v", got)
	}
	got.PastRemoteIDs[0] = "external"
	again, _ := s.Get("a")
	if again.PastRemoteIDs[0] != "old" {
		t.Fatal("read result aliases stored data")
	}
}

func TestCompetingUpdatesSurviveRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "jobs.json")
	s, _ := Open(path)
	if err := s.Put(job.Job{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.Update("a", func(j *job.Job) error { j.Attempts++; return nil })
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := reopened.Get("a")
	if got.Attempts != 12 {
		t.Fatalf("lost competing update: %d", got.Attempts)
	}
}
