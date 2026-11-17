// overmynd-db performs explicit jobs-only maintenance on development databases.
// It never loads application settings, starts the pipeline or contacts services.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/z354prance/overmynd/internal/store"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "overmynd-db:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: overmynd-db import|backup -database PATH [-snapshot CAPTURE | -output BACKUP]")
	}
	command := args[0]
	if command != "import" && command != "backup" {
		return fmt.Errorf("unknown command %q", command)
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", "", "explicit development SQLite path")
	var snapshot, output string
	if command == "import" {
		flags.StringVar(&snapshot, "snapshot", "", "immutable captured JSON file")
	} else {
		flags.StringVar(&output, "output", "", "new backup file")
	}
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || *database == "" || (command == "import" && snapshot == "") || (command == "backup" && output == "") {
		return errors.New("explicit database and snapshot/output paths are required; positional arguments are not accepted")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if command == "import" {
		// Read a separately captured file, never discover a running app's store.
		data, err := readCapture(snapshot)
		if err != nil {
			return err
		}
		s, err := store.OpenSQLite(ctx, *database)
		if err != nil {
			return err
		}
		defer s.Close()
		imported, err := s.ImportSnapshot(ctx, data)
		if err != nil {
			return err
		}
		// Only report success after the transaction and error-aware read complete.
		jobs, err := s.List(ctx)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(struct {
			Imported bool `json:"imported"`
			Jobs     int  `json:"jobs"`
		}{imported, len(jobs)})
	}
	// A typo in the source must not create an empty database and back it up.
	if err := requireSQLite(*database); err != nil {
		return err
	}
	s, err := store.OpenSQLite(ctx, *database)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Backup(ctx, output); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(struct {
		BackedUp bool `json:"backedUp"`
	}{true})
}

func readCapture(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("snapshot must be a regular captured file")
	}
	return os.ReadFile(path)
}

func requireSQLite(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("database must be a regular SQLite file")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	header := make([]byte, 16)
	if _, err := io.ReadFull(f, header); err != nil {
		return err
	}
	if string(header) != "SQLite format 3\x00" {
		return errors.New("source is not a SQLite database")
	}
	return nil
}
