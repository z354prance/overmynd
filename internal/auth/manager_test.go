package auth

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/z354prance/overmynd/internal/database"
)

func testManager(t *testing.T) *Manager {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return NewManager(db)
}

func TestSetupLoginAuthenticateLogout(t *testing.T) {
	manager := testManager(t)

	required, err := manager.SetupRequired()
	if err != nil {
		t.Fatalf("setup required: %v", err)
	}

	if !required {
		t.Fatal("setup should initially be required")
	}

	setupSession, err := manager.Setup(
		"admin",
		"correct horse battery staple",
	)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	if setupSession.Token == "" {
		t.Fatal("setup should return a session token")
	}

	required, err = manager.SetupRequired()
	if err != nil {
		t.Fatalf("setup required after setup: %v", err)
	}

	if required {
		t.Fatal("setup should no longer be required")
	}

	if _, err := manager.Setup(
		"other",
		"another secure password",
	); !errors.Is(err, ErrSetupComplete) {
		t.Fatalf("expected setup complete error, got %v", err)
	}

	user, err := manager.Authenticate(setupSession.Token)
	if err != nil {
		t.Fatalf("authenticate setup session: %v", err)
	}

	if user.Username != "admin" {
		t.Fatalf("unexpected username: %s", user.Username)
	}

	if _, err := manager.Login(
		"admin",
		"wrong password",
	); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected invalid credentials, got %v", err)
	}

	loginSession, err := manager.Login(
		"admin",
		"correct horse battery staple",
	)
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	if loginSession.Token == setupSession.Token {
		t.Fatal("sessions should use unique tokens")
	}

	if err := manager.Logout(loginSession.Token); err != nil {
		t.Fatalf("logout: %v", err)
	}

	if _, err := manager.Authenticate(
		loginSession.Token,
	); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("expected logged-out session to fail, got %v", err)
	}

	if _, err := manager.Authenticate(
		setupSession.Token,
	); err != nil {
		t.Fatalf("other session should remain valid: %v", err)
	}
}

func TestSetupValidation(t *testing.T) {
	manager := testManager(t)

	if _, err := manager.Setup(
		"ab",
		"correct horse battery staple",
	); err == nil {
		t.Fatal("short username should fail")
	}

	if _, err := manager.Setup(
		"admin",
		"short",
	); err == nil {
		t.Fatal("short password should fail")
	}
}

func TestPasswordHash(t *testing.T) {
	encoded, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}

	if encoded == "correct horse battery staple" {
		t.Fatal("password must not be stored directly")
	}

	matches, err := verifyPassword(
		"correct horse battery staple",
		encoded,
	)
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}

	if !matches {
		t.Fatal("correct password should match")
	}

	matches, err = verifyPassword("wrong password", encoded)
	if err != nil {
		t.Fatalf("verify wrong password: %v", err)
	}

	if matches {
		t.Fatal("wrong password should not match")
	}
}
