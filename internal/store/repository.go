package store

import "github.com/z354prance/overmynd/internal/job"

// LegacyRepository is the inherited snapshot API. It is deliberately separate
// from the error-aware SQLite repository; do not adapt database errors to false
// or an empty list when the application migrates to that repository.
type LegacyRepository interface {
	Get(id string) (job.Job, bool)
	List() []job.Job
	Put(job.Job) error
	Update(id string, fn func(*job.Job) error) (job.Job, error)
}

var _ LegacyRepository = (*Store)(nil)
