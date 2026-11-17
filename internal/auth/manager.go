package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/database"
	"golang.org/x/crypto/argon2"
)

const (
	sessionLifetime = 30 * 24 * time.Hour

	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrSetupComplete      = errors.New("administrator setup is already complete")
	ErrNotAuthenticated   = errors.New("not authenticated")
)

type Manager struct {
	db *database.Database
}

type Session struct {
	Token     string
	ExpiresAt time.Time
	User      database.User
}

func NewManager(db *database.Database) *Manager {
	return &Manager{db: db}
}

func (m *Manager) SetupRequired() (bool, error) {
	count, err := m.db.UserCount()
	if err != nil {
		return false, err
	}

	return count == 0, nil
}

func (m *Manager) Setup(
	username string,
	password string,
) (Session, error) {
	username = strings.TrimSpace(username)

	if err := validateUsername(username); err != nil {
		return Session{}, err
	}

	if err := validatePassword(password); err != nil {
		return Session{}, err
	}

	passwordHash, err := hashPassword(password)
	if err != nil {
		return Session{}, err
	}

	userID, created, err := m.db.CreateInitialUser(username, passwordHash)
	if err != nil {
		return Session{}, err
	}
	if !created {
		return Session{}, ErrSetupComplete
	}

	user, err := m.db.GetUser(userID)
	if err != nil {
		return Session{}, err
	}

	return m.createSession(user)
}

func (m *Manager) Login(
	username string,
	password string,
) (Session, error) {
	user, err := m.db.GetUserByUsername(strings.TrimSpace(username))
	if err != nil {
		return Session{}, ErrInvalidCredentials
	}

	matches, err := verifyPassword(password, user.PasswordHash)
	if err != nil {
		return Session{}, ErrInvalidCredentials
	}

	if !matches {
		return Session{}, ErrInvalidCredentials
	}

	return m.createSession(user)
}

func (m *Manager) Authenticate(token string) (database.User, error) {
	if token == "" {
		return database.User{}, ErrNotAuthenticated
	}

	tokenHash := hashToken(token)

	session, err := m.db.GetSessionByTokenHash(tokenHash)
	if err != nil {
		return database.User{}, ErrNotAuthenticated
	}

	if !session.ExpiresAt.After(time.Now().UTC()) {
		_ = m.db.DeleteSessionByTokenHash(tokenHash)
		return database.User{}, ErrNotAuthenticated
	}

	user, err := m.db.GetUser(session.UserID)
	if err != nil {
		return database.User{}, ErrNotAuthenticated
	}

	return user, nil
}

func (m *Manager) Logout(token string) error {
	if token == "" {
		return nil
	}

	return m.db.DeleteSessionByTokenHash(hashToken(token))
}

func (m *Manager) CleanupExpiredSessions() error {
	return m.db.DeleteExpiredSessions(time.Now().UTC())
}

func (m *Manager) createSession(user database.User) (Session, error) {
	tokenBytes := make([]byte, 32)

	if _, err := rand.Read(tokenBytes); err != nil {
		return Session{}, fmt.Errorf("generate session token: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	expiresAt := time.Now().UTC().Add(sessionLifetime)

	if err := m.db.CreateSession(
		user.ID,
		hashToken(token),
		expiresAt,
	); err != nil {
		return Session{}, err
	}

	return Session{
		Token:     token,
		ExpiresAt: expiresAt,
		User:      user,
	}, nil
}

func validateUsername(username string) error {
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}

	if len(username) > 64 {
		return fmt.Errorf("username must be 64 characters or fewer")
	}

	return nil
}

func validatePassword(password string) error {
	if len(password) < 12 {
		return fmt.Errorf("password must be at least 12 characters")
	}

	if len(password) > 1024 {
		return fmt.Errorf("password is too long")
	}

	return nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)

	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt,
		argonIterations,
		argonMemory,
		argonParallelism,
		argonKeyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory,
		argonIterations,
		argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func verifyPassword(password string, encoded string) (bool, error) {
	var (
		memory      uint32
		iterations  uint32
		parallelism uint8
	)

	parts := strings.Split(encoded, "$")

	if len(parts) != 6 ||
		parts[1] != "argon2id" ||
		parts[2] != "v=19" {
		return false, fmt.Errorf("invalid password hash")
	}

	if _, err := fmt.Sscanf(
		parts[3],
		"m=%d,t=%d,p=%d",
		&memory,
		&iterations,
		&parallelism,
	); err != nil {
		return false, fmt.Errorf("parse password parameters: %w", err)
	}

	// Refuse corrupted or malicious parameters before allocating memory.
	if memory == 0 || memory > argonMemory ||
		iterations == 0 || iterations > argonIterations ||
		parallelism == 0 || parallelism > argonParallelism {
		return false, fmt.Errorf("invalid password parameters")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode password salt: %w", err)
	}

	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode password hash: %w", err)
	}
	if len(salt) == 0 || len(salt) > 64 || len(expected) == 0 || len(expected) > 64 {
		return false, fmt.Errorf("invalid password hash sizes")
	}

	actual := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memory,
		parallelism,
		uint32(len(expected)),
	)

	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
