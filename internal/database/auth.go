package database

import (
	"database/sql"
	"fmt"
	"time"
)

type User struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	CreatedAt time.Time
	ExpiresAt time.Time
}

func (d *Database) UserCount() (int, error) {
	var count int

	if err := d.DB.QueryRow(`
		SELECT COUNT(*)
		FROM users
	`).Scan(&count); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}

	return count, nil
}

func (d *Database) CreateUser(
	username string,
	passwordHash string,
) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := d.DB.Exec(`
		INSERT INTO users (
			username,
			password_hash,
			created_at,
			updated_at
		)
		VALUES (?, ?, ?, ?)
	`,
		username,
		passwordHash,
		now,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("read user id: %w", err)
	}

	return id, nil
}

func (d *Database) GetUserByUsername(username string) (User, error) {
	row := d.DB.QueryRow(`
		SELECT
			id,
			username,
			password_hash,
			created_at,
			updated_at
		FROM users
		WHERE username = ?
	`, username)

	return scanUser(row)
}

func (d *Database) GetUser(id int64) (User, error) {
	row := d.DB.QueryRow(`
		SELECT
			id,
			username,
			password_hash,
			created_at,
			updated_at
		FROM users
		WHERE id = ?
	`, id)

	return scanUser(row)
}

func (d *Database) CreateSession(
	userID int64,
	tokenHash string,
	expiresAt time.Time,
) error {
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := d.DB.Exec(`
		INSERT INTO sessions (
			user_id,
			token_hash,
			created_at,
			expires_at
		)
		VALUES (?, ?, ?, ?)
	`,
		userID,
		tokenHash,
		now,
		expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	return nil
}

func (d *Database) GetSessionByTokenHash(
	tokenHash string,
) (Session, error) {
	row := d.DB.QueryRow(`
		SELECT
			id,
			user_id,
			token_hash,
			created_at,
			expires_at
		FROM sessions
		WHERE token_hash = ?
	`, tokenHash)

	var (
		session   Session
		createdAt string
		expiresAt string
	)

	err := row.Scan(
		&session.ID,
		&session.UserID,
		&session.TokenHash,
		&createdAt,
		&expiresAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return Session{}, fmt.Errorf("session not found")
		}

		return Session{}, fmt.Errorf("scan session: %w", err)
	}

	session.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return Session{}, fmt.Errorf(
			"parse session created time: %w",
			err,
		)
	}

	session.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return Session{}, fmt.Errorf(
			"parse session expiry time: %w",
			err,
		)
	}

	return session, nil
}

func (d *Database) DeleteSessionByTokenHash(tokenHash string) error {
	if _, err := d.DB.Exec(`
		DELETE FROM sessions
		WHERE token_hash = ?
	`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

func (d *Database) DeleteExpiredSessions(now time.Time) error {
	if _, err := d.DB.Exec(`
		DELETE FROM sessions
		WHERE expires_at <= ?
	`, now.UTC().Format(time.RFC3339)); err != nil {
		return fmt.Errorf("delete expired sessions: %w", err)
	}

	return nil
}

type userScanner interface {
	Scan(dest ...any) error
}

func scanUser(row userScanner) (User, error) {
	var (
		user      User
		createdAt string
		updatedAt string
	)

	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.PasswordHash,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return User{}, fmt.Errorf("user not found")
		}

		return User{}, fmt.Errorf("scan user: %w", err)
	}

	user.CreatedAt, err = time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return User{}, fmt.Errorf(
			"parse user created time: %w",
			err,
		)
	}

	user.UpdatedAt, err = time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return User{}, fmt.Errorf(
			"parse user updated time: %w",
			err,
		)
	}

	return user, nil
}
