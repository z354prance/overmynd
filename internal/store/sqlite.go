package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/z354prance/overmynd/internal/job"
	_ "modernc.org/sqlite"
)

// Repository distinguishes absent jobs from storage failures. Update callbacks
// must only edit the job: they run in a short transaction and must not perform IO.
type Repository interface {
	Get(context.Context, string) (job.Job, error)
	List(context.Context) ([]job.Job, error)
	Put(context.Context, job.Job) error
	Update(context.Context, string, func(*job.Job) error) (job.Job, error)
}

// SQLite is an opt-in development repository. The application continues using
// JSON until migration and error propagation through its callers are complete.
type SQLite struct{ db *sql.DB }

var _ Repository = (*SQLite)(nil)

func OpenSQLite(ctx context.Context, path string) (*SQLite, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Create privately before SQLite opens the file (WAL inherits its mode).
	f, err := os.OpenFile(absolute, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: absolute}
	q := u.Query()
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	// Serialize this handle's writers; independent handles are coordinated by SQLite.
	db.SetMaxOpenConns(1)
	s := &SQLite{db: db}
	fail := func(err error) (*SQLite, error) { db.Close(); return nil, err }
	var mode string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&mode); err != nil {
		return fail(err)
	}
	if mode != "wal" {
		return fail(fmt.Errorf("unexpected journal mode %q", mode))
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fail(err)
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(fmt.Errorf("unsupported schema version %d", version))
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `
 CREATE TABLE jobs (id TEXT PRIMARY KEY, state TEXT NOT NULL, catalog_id TEXT, payload BLOB NOT NULL);
 CREATE INDEX jobs_state ON jobs(state);
 CREATE TABLE transitions (sequence INTEGER PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), payload BLOB NOT NULL);
 CREATE TABLE import_intents (id TEXT PRIMARY KEY, job_id TEXT NOT NULL REFERENCES jobs(id), source TEXT NOT NULL, destination TEXT NOT NULL, state TEXT NOT NULL CHECK(state IN ('identified','planned','copying','finalized','verified','needs_review','conflict','failed')), detail TEXT NOT NULL DEFAULT '');
 CREATE INDEX import_intents_job ON import_intents(job_id);
 PRAGMA user_version=1;`)
		if err != nil {
			return fail(err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fail(err)
	}
	return s, nil
}

func (s *SQLite) Close() error { return s.db.Close() }

type scanner interface{ Scan(...any) error }

func readJob(row scanner) (job.Job, error) {
	var data []byte
	if err := row.Scan(&data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return job.Job{}, ErrNotFound
		}
		return job.Job{}, err
	}
	var j job.Job
	err := json.Unmarshal(data, &j)
	return j, err
}
func (s *SQLite) Get(ctx context.Context, id string) (job.Job, error) {
	return readJob(s.db.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id=?", id))
}
func (s *SQLite) List(ctx context.Context) ([]job.Job, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM jobs ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []job.Job{}
	for rows.Next() {
		j, err := readJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// Put inserts a job exactly once. Changes use Update so history cannot be lost
// through a stale snapshot replacing a concurrent update.
func (s *SQLite) Put(ctx context.Context, j job.Job) error {
	if j.ID == "" {
		return errors.New("empty job ID")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	data, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO jobs(id,state,payload) VALUES(?,?,?)", j.ID, j.State, data); err != nil {
		return err
	}
	if err := appendTransitions(ctx, tx, j.ID, j.History); err != nil {
		return err
	}
	return tx.Commit()
}
func appendTransitions(ctx context.Context, tx *sql.Tx, id string, history []job.Transition) error {
	for _, t := range history {
		data, err := json.Marshal(t)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO transitions(job_id,payload) VALUES(?,?)", id, data); err != nil {
			return err
		}
	}
	return nil
}
func (s *SQLite) Update(ctx context.Context, id string, fn func(*job.Job) error) (job.Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return job.Job{}, err
	}
	defer tx.Rollback()
	j, err := readJob(tx.QueryRowContext(ctx, "SELECT payload FROM jobs WHERE id=?", id))
	if err != nil {
		return job.Job{}, err
	}
	// The payload carries a bounded history. Durable history is appended from the
	// callback's newly recorded transitions, including when Move trims the payload.
	before := append([]job.Transition(nil), j.History...)
	if err := fn(&j); err != nil {
		return job.Job{}, err
	}
	if j.ID != id {
		return job.Job{}, errors.New("job ID cannot change")
	}
	// Locate the overlap after the callback; reject edits to historical records.
	overlap := len(before)
	for overlap > 0 && !historyPrefix(j.History, before[len(before)-overlap:]) {
		overlap--
	}
	if len(before) > 0 && overlap == 0 {
		return job.Job{}, errors.New("existing history cannot be rewritten")
	}
	data, err := json.Marshal(j)
	if err != nil {
		return job.Job{}, err
	}
	if _, err := tx.ExecContext(ctx, "UPDATE jobs SET state=?,payload=? WHERE id=?", j.State, data, id); err != nil {
		return job.Job{}, err
	}
	if err := appendTransitions(ctx, tx, id, j.History[overlap:]); err != nil {
		return job.Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return job.Job{}, err
	}
	return j.Clone(), nil
}
func historyPrefix(history, prefix []job.Transition) bool {
	if len(history) < len(prefix) {
		return false
	}
	for i := range prefix {
		if history[i] != prefix[i] {
			return false
		}
	}
	return true
}

// ImportIntent records a per-file fact independently of job cleanup. No file IO
// occurs here, and verified must only be recorded after actual verification.
type ImportIntent struct{ ID, JobID, Source, Destination, State, Detail string }

func (s *SQLite) SaveIntent(ctx context.Context, i ImportIntent) error {
	if i.ID == "" || i.Source == "" || i.Destination == "" {
		return errors.New("incomplete import intent")
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO import_intents(id,job_id,source,destination,state,detail) VALUES(?,?,?,?,?,?)
 ON CONFLICT(id) DO UPDATE SET state=excluded.state,detail=excluded.detail
 WHERE import_intents.job_id=excluded.job_id AND import_intents.source=excluded.source AND import_intents.destination=excluded.destination`, i.ID, i.JobID, i.Source, i.Destination, i.State, i.Detail)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errors.New("import intent identity cannot change")
	}
	return nil
}
func (s *SQLite) Intents(ctx context.Context, jobID string) ([]ImportIntent, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id,job_id,source,destination,state,detail FROM import_intents WHERE job_id=? ORDER BY id", jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ImportIntent{}
	for rows.Next() {
		var i ImportIntent
		if err := rows.Scan(&i.ID, &i.JobID, &i.Source, &i.Destination, &i.State, &i.Detail); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// Transitions returns the full committed journal, beyond the payload's display limit.
func (s *SQLite) Transitions(ctx context.Context, id string) ([]job.Transition, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT payload FROM transitions WHERE job_id=? ORDER BY sequence", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []job.Transition{}
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var t job.Transition
		if err := json.Unmarshal(data, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
