package database

import (
	"database/sql"
	"errors"
	"time"
)

func (d *Database) GetSetting(key string) (string, error) {
	var value string
	err := d.DB.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (d *Database) SetSetting(key, value string) error {
	_, err := d.DB.Exec(`INSERT INTO settings(key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, time.Now().UTC().Format(time.RFC3339))
	return err
}
