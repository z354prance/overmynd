package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/z354prance/overmynd/internal/job"
)

// ImportSnapshot imports bytes from an immutable, separately captured JSON backup.
// It never opens the source file or contacts a backend. Keep the original backup,
// settings and secret.key separately; this operation migrates jobs only.
// The returned boolean is false when this exact snapshot was already committed.
func (s *SQLite) ImportSnapshot(ctx context.Context, snapshot []byte) (bool, error) {
	var jobs []*job.Job
	if err := json.Unmarshal(snapshot, &jobs); err != nil {
		return false, fmt.Errorf("decode job snapshot: %w", err)
	}
	if jobs == nil {
		return false, errors.New("job snapshot must be an array")
	}
	seen := make(map[string]bool, len(jobs))
	for _, j := range jobs {
		if j == nil || j.ID == "" {
			return false, errors.New("snapshot contains null job or empty ID")
		}
		if seen[j.ID] {
			return false, fmt.Errorf("duplicate snapshot job %q", j.ID)
		}
		seen[j.ID] = true
		if !snapshotState(j.State) {
			return false, fmt.Errorf("unknown state for job %q: %q", j.ID, j.State)
		}
		for n, tr := range j.History {
			if !snapshotState(tr.From) || !snapshotState(tr.To) || (n > 0 && j.History[n-1].To != tr.From) {
				return false, fmt.Errorf("invalid history for job %q", j.ID)
			}
		}
		if len(j.History) > 0 && j.History[len(j.History)-1].To != j.State {
			return false, fmt.Errorf("history disagrees with state for job %q", j.ID)
		}
	}
	digest := sha256.Sum256(snapshot)
	fingerprint := hex.EncodeToString(digest[:])
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	// Created transactionally so older schema-v1 development databases also work.
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS snapshot_import (singleton INTEGER PRIMARY KEY CHECK(singleton=1), sha256 TEXT NOT NULL, job_count INTEGER NOT NULL)`); err != nil {
		return false, err
	}
	var previous string
	err = tx.QueryRowContext(ctx, "SELECT sha256 FROM snapshot_import WHERE singleton=1").Scan(&previous)
	if err == nil {
		if previous != fingerprint {
			return false, errors.New("a different snapshot was already imported")
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM jobs").Scan(&count); err != nil {
		return false, err
	}
	if count != 0 {
		return false, errors.New("snapshot import requires an empty job repository")
	}
	for _, j := range jobs {
		payload, err := json.Marshal(j)
		if err != nil {
			return false, err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO jobs(id,state,payload) VALUES(?,?,?)", j.ID, j.State, payload); err != nil {
			return false, err
		}
		if err := appendTransitions(ctx, tx, j.ID, j.History); err != nil {
			return false, err
		}
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO snapshot_import(singleton,sha256,job_count) VALUES(1,?,?)", fingerprint, len(jobs)); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func snapshotState(s job.State) bool {
	switch s {
	case job.Received, job.Submitted, job.Downloading, job.Pulling, job.Downloaded,
		job.QueuedTranscode, job.Transcoding, job.Verifying, job.Ready, job.Imported, job.Cleaned, job.Failed:
		return true
	}
	return false
}
