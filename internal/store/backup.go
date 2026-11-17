package store

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// Backup writes a consistent jobs-only snapshot, including WAL-backed committed
// data, schema, transition journal, import intents and snapshot-import marker.
// It refuses existing destinations. Settings and secret.key must be preserved
// separately before this can be called a full operational backup.
func (s *SQLite) Backup(ctx context.Context, destination string) error {
	if destination == "" {
		return errors.New("empty backup destination")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	path, err := filepath.Abs(destination)
	if err != nil {
		return err
	}
	// VACUUM INTO accepts an empty file. Claim it exclusively and privately,
	// rather than checking existence then risking replacement of someone's file.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(path)
		return err
	}
	complete := false
	defer func() {
		if !complete {
			os.Remove(path)
		}
	}()
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO ?", path); err != nil {
		return err
	}
	// Explicitly flush the completed backup before reporting success.
	out, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	err = out.Sync()
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	complete = true
	return nil
}
