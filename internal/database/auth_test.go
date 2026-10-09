package database

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestInitialUserAcrossConnections(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if err := first.Migrate(); err != nil {
		t.Fatal(err)
	}
	second, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	start := make(chan struct{})
	results := make(chan bool, 16)
	for i := 0; i < 16; i++ {
		go func(i int) {
			<-start
			db := first
			if i%2 == 0 {
				db = second
			}
			_, created, err := db.CreateInitialUser(fmt.Sprintf("admin%d", i), "hash")
			if err != nil {
				t.Errorf("create: %v", err)
			}
			results <- created
		}(i)
	}
	close(start)
	winners := 0
	for i := 0; i < 16; i++ {
		if <-results {
			winners++
		}
	}
	count, err := first.UserCount()
	if winners != 1 || count != 1 || err != nil {
		t.Fatalf("winners=%d users=%d err=%v", winners, count, err)
	}
}

func TestAuthRepository(t *testing.T) {
	db := testDatabase(t)

	count, err := db.UserCount()
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no users, got %d", count)
	}

	userID, err := db.CreateUser("admin", "argon2id-test-hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	count, err = db.UserCount()
	if err != nil {
		t.Fatalf("count users after create: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 user, got %d", count)
	}

	user, err := db.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("get user by username: %v", err)
	}
	if user.ID != userID {
		t.Fatalf("unexpected user id: got %d want %d", user.ID, userID)
	}
	if user.PasswordHash != "argon2id-test-hash" {
		t.Fatal("password hash mismatch")
	}

	userByID, err := db.GetUser(userID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if userByID.Username != "admin" {
		t.Fatalf("unexpected username: %s", userByID.Username)
	}

	expiresAt := time.Now().UTC().Add(30 * 24 * time.Hour)
	if err := db.CreateSession(userID, "session-token-hash", expiresAt); err != nil {
		t.Fatalf("create session: %v", err)
	}

	session, err := db.GetSessionByTokenHash("session-token-hash")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.UserID != userID {
		t.Fatalf("unexpected session user: got %d want %d", session.UserID, userID)
	}
	if session.TokenHash != "session-token-hash" {
		t.Fatal("session token hash mismatch")
	}
	if !session.ExpiresAt.Equal(expiresAt.Truncate(time.Second)) {
		t.Fatalf("unexpected expiry: got %v want %v", session.ExpiresAt, expiresAt.Truncate(time.Second))
	}

	if err := db.DeleteSessionByTokenHash("session-token-hash"); err != nil {
		t.Fatalf("delete session: %v", err)
	}
	if _, err := db.GetSessionByTokenHash("session-token-hash"); err == nil {
		t.Fatal("deleted session should not exist")
	}
}

func TestCreateInitialUserAtomic(t *testing.T) {
	db := testDatabase(t)

	const attempts = 8
	var wg sync.WaitGroup
	results := make(chan bool, attempts)
	errs := make(chan error, attempts)

	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, created, err := db.CreateInitialUser("admin", "argon2id-test-hash")
			if err != nil {
				errs <- err
				return
			}
			results <- created
		}()
	}
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Fatalf("create initial user: %v", err)
	}
	createdCount := 0
	for created := range results {
		if created {
			createdCount++
		}
	}
	if createdCount != 1 {
		t.Fatalf("expected exactly one initial user creation, got %d", createdCount)
	}

	count, err := db.UserCount()
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one user, got %d", count)
	}
}

func TestDeleteExpiredSessions(t *testing.T) {
	db := testDatabase(t)
	userID, err := db.CreateUser("admin", "argon2id-test-hash")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	now := time.Now().UTC()
	if err := db.CreateSession(userID, "expired-token", now.Add(-time.Hour)); err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	if err := db.CreateSession(userID, "active-token", now.Add(time.Hour)); err != nil {
		t.Fatalf("create active session: %v", err)
	}
	if err := db.DeleteExpiredSessions(now); err != nil {
		t.Fatalf("delete expired sessions: %v", err)
	}
	if _, err := db.GetSessionByTokenHash("expired-token"); err == nil {
		t.Fatal("expired session should have been deleted")
	}
	if _, err := db.GetSessionByTokenHash("active-token"); err != nil {
		t.Fatalf("active session should remain: %v", err)
	}
}
