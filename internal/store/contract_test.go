package store

import (
	"context"
	"errors"
	"github.com/z354prance/overmynd/internal/job"
	"path/filepath"
	"testing"
	"time"
)

func TestRepositoryReadWriteContract(t *testing.T) {
	for _, kind := range []string{"json", "sqlite"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			var repo Repository
			if kind == "json" {
				source, err := Open(filepath.Join(t.TempDir(), "jobs.json"))
				if err != nil {
					t.Fatal(err)
				}
				repo = FromLegacy(source)
			} else {
				s, err := OpenSQLite(ctx, filepath.Join(t.TempDir(), "jobs.db"))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { s.Close() })
				repo = s
			}
			if _, err := repo.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
				t.Fatal(err)
			}
			first := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
			for _, j := range []job.Job{{ID: "a-old", State: job.Received, Created: first, PastRemoteIDs: []string{"original"}}, {ID: "z-new", State: job.Received, Created: first.Add(time.Hour).In(time.FixedZone("offset", -6*3600))}} {
				if err := repo.Put(ctx, j); err != nil {
					t.Fatal(err)
				}
			}
			jobs, err := repo.List(ctx)
			if err != nil || len(jobs) != 2 || jobs[0].ID != "z-new" {
				t.Fatal("queue ordering changed", jobs, err)
			}
			got, err := repo.Get(ctx, "a-old")
			if err != nil {
				t.Fatal(err)
			}
			got.PastRemoteIDs[0] = "changed outside storage"
			fail := errors.New("callback failed")
			if _, err = repo.Update(ctx, "a-old", func(j *job.Job) error { j.PastRemoteIDs[0] = "failed change"; return fail }); !errors.Is(err, fail) {
				t.Fatal(err)
			}
			if _, err = repo.Update(ctx, "a-old", func(j *job.Job) error { j.ID = "different"; return nil }); err == nil {
				t.Fatal("job identity changed")
			}
			got, err = repo.Get(ctx, "a-old")
			if err != nil || got.ID != "a-old" || got.PastRemoteIDs[0] != "original" {
				t.Fatal("read or failed-update alias escaped", got, err)
			}
		})
	}
}
