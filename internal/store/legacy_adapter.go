package store

import (
	"context"
	"github.com/z354prance/overmynd/internal/job"
)

// FromLegacy lets migrated callers use the error-aware contract while the
// executable still opens JSON. SQLite must implement Repository directly; it
// must never be wrapped to turn read failures into a legacy missing result.
func FromLegacy(s LegacyRepository) Repository { return legacyAdapter{s} }

type legacyAdapter struct{ source LegacyRepository }

func (a legacyAdapter) Get(ctx context.Context, id string) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}
	j, ok := a.source.Get(id)
	if !ok {
		return job.Job{}, ErrNotFound
	}
	return j, nil
}
func (a legacyAdapter) List(ctx context.Context) ([]job.Job, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return a.source.List(), nil
}
func (a legacyAdapter) Put(ctx context.Context, j job.Job) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return a.source.Put(j)
}
func (a legacyAdapter) Update(ctx context.Context, id string, fn func(*job.Job) error) (job.Job, error) {
	if err := ctx.Err(); err != nil {
		return job.Job{}, err
	}
	return a.source.Update(id, func(j *job.Job) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(j); err != nil {
			return err
		}
		return ctx.Err()
	})
}
