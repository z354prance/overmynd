// Package store keeps jobs in memory and persists them with an atomic write
// after every change, so a restart resumes from the last stored state.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

var ErrNotFound = errors.New("job not found")

type Store struct {
	mu   sync.Mutex
	path string
	jobs map[string]*job.Job
	Now  func() time.Time
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, jobs: map[string]*job.Job{}, Now: time.Now}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*job.Job
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, j := range list {
		s.jobs[j.ID] = j
	}
	return s, nil
}

func (s *Store) saveLocked() error {
	// Drop long-finished jobs so the file stays small.
	cutoff := s.Now().Add(-7 * 24 * time.Hour)
	list := make([]*job.Job, 0, len(s.jobs))
	for id, j := range s.jobs {
		if j.State == job.Cleaned && j.Updated.Before(cutoff) {
			delete(s.jobs, id)
			continue
		}
		list = append(list, j)
	}
	sort.Slice(list, func(a, b int) bool { return list[a].Created.Before(list[b].Created) })
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) Get(id string) (job.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return job.Job{}, false
	}
	return j.Clone(), true
}

func (s *Store) List() []job.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]job.Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		out = append(out, j.Clone())
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Created.After(out[b].Created) })
	return out
}

// Put inserts or replaces a job.
func (s *Store) Put(j job.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := j.Clone()
	s.jobs[j.ID] = &c
	return s.saveLocked()
}

// Update applies fn to the stored job and persists the result. If fn returns an
// error nothing is saved.
func (s *Store) Update(id string, fn func(*job.Job) error) (job.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.jobs[id]
	if !ok {
		return job.Job{}, ErrNotFound
	}
	c := cur.Clone()
	if err := fn(&c); err != nil {
		return job.Job{}, err
	}
	*cur = c
	if err := s.saveLocked(); err != nil {
		return job.Job{}, err
	}
	return c.Clone(), nil
}
