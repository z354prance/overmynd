package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Database struct {
	DB   *sql.DB
	path string
}

func Open(configDir string) (*Database, error) {
	if err := os.MkdirAll(configDir, 0750); err != nil {
		return nil, fmt.Errorf("create config directory: %w", err)
	}

	path := filepath.Join(configDir, "overmynd.db")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA synchronous = NORMAL;",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("apply database pragma: %w", err)
		}
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Database{
		DB:   db,
		path: path,
	}, nil
}

func (d *Database) Close() error {
	return d.DB.Close()
}

func (d *Database) Path() string {
	return d.path
}

func (d *Database) Migrate() error {
	if _, err := d.DB.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TEXT NOT NULL
		);
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	var current int

	err := d.DB.QueryRow(`
		SELECT COALESCE(MAX(version), 0)
		FROM schema_migrations
	`).Scan(&current)
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}

	migrations := []struct {
		version int
		sql     string
	}{
		{
			version: 1,
			sql: `
				CREATE TABLE settings (
					key TEXT PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TEXT NOT NULL
				);

				CREATE TABLE services (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					type TEXT NOT NULL,
					name TEXT NOT NULL,
					enabled INTEGER NOT NULL DEFAULT 1,
					base_url TEXT NOT NULL,
					credentials TEXT NOT NULL DEFAULT '',
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL,
					UNIQUE(type, name)
				);

				CREATE TABLE service_health (
					service_id INTEGER PRIMARY KEY,
					status TEXT NOT NULL DEFAULT 'unknown',
					message TEXT NOT NULL DEFAULT '',
					last_check_at TEXT,
					last_success_at TEXT,
					FOREIGN KEY(service_id)
						REFERENCES services(id)
						ON DELETE CASCADE
				);

				CREATE TABLE users (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					username TEXT NOT NULL UNIQUE,
					password_hash TEXT NOT NULL,
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				);
			`,
		},
		{
			version: 2,
			sql: `
                                CREATE TABLE sessions (
                                        id INTEGER PRIMARY KEY AUTOINCREMENT,
                                        user_id INTEGER NOT NULL,
                                        token_hash TEXT NOT NULL UNIQUE,
                                        created_at TEXT NOT NULL,
                                        expires_at TEXT NOT NULL,
                                        FOREIGN KEY(user_id)
                                                REFERENCES users(id)
                                                ON DELETE CASCADE
                                );

                                CREATE INDEX idx_sessions_expires_at
                                        ON sessions(expires_at);
                        `,
		},
	}

	for _, migration := range migrations {
		if migration.version <= current {
			continue
		}

		tx, err := d.DB.Begin()
		if err != nil {
			return fmt.Errorf(
				"begin migration %d: %w",
				migration.version,
				err,
			)
		}

		if _, err := tx.Exec(migration.sql); err != nil {
			tx.Rollback()
			return fmt.Errorf(
				"run migration %d: %w",
				migration.version,
				err,
			)
		}

		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at)
			 VALUES(?, ?)`,
			migration.version,
			time.Now().UTC().Format(time.RFC3339),
		); err != nil {
			tx.Rollback()
			return fmt.Errorf(
				"record migration %d: %w",
				migration.version,
				err,
			)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf(
				"commit migration %d: %w",
				migration.version,
				err,
			)
		}
	}

	return nil
}
