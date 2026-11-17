package auth

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/z354prance/overmynd/internal/secret"
)

func TestTOTPVector(t *testing.T) {
	// RFC 6238 appendix B, SHA-1, T=59s: 94287082 (8 digits) => 287082 as 6 digits.
	sec := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ" // base32 of "12345678901234567890"
	got, ok := totpAt(sec, time.Unix(59, 0), 8)
	if !ok || got != "94287082" {
		t.Fatalf("got %q", got)
	}
	got, _ = totpAt(sec, time.Unix(59, 0), 6)
	if got != "287082" {
		t.Fatalf("got %q", got)
	}
}

func TestPasswordHash(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("correct horse battery", h) || VerifyPassword("wrong", h) {
		t.Fatal("verify failed")
	}
}

func newAuth(t *testing.T) (*Auth, *time.Time) {
	secret.SetKeyForTest(make([]byte, 32))
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	a.Now = func() time.Time { return now }
	return a, &now
}

func TestSetupAndLogin(t *testing.T) {
	a, now := newAuth(t)
	if _, err := a.Setup("wrong-key", "a long enough password"); err != ErrBadSetupKey {
		t.Fatalf("setup without the key must fail: %v", err)
	}
	res, err := a.Setup(a.SetupKey(), "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Setup(a.setupKey, "another password!!"); err != ErrExists {
		t.Fatalf("second setup must fail: %v", err)
	}
	if _, err := a.Login("ip", "a long enough password", "000000"); err != ErrNoAccount {
		t.Fatalf("login before confirming the authenticator must fail: %v", err)
	}
	code, _ := totpAt(res.TOTPSecret, *now, 6)
	rec, err := a.Confirm("a long enough password", code)
	if err != nil || len(rec) != 8 {
		t.Fatalf("confirm: %v %v", err, rec)
	}
	*now = now.Add(31 * time.Second)
	code, _ = totpAt(res.TOTPSecret, *now, 6)
	tok, err := a.Login("ip", "a long enough password", code)
	if err != nil || !a.Valid(tok) {
		t.Fatalf("login: %v", err)
	}
	a.Logout(tok)
	if a.Valid(tok) {
		t.Fatal("logout must end the session")
	}
	// recovery code works once
	if _, err := a.Login("ip", "a long enough password", rec[0]); err != nil {
		t.Fatalf("recovery code: %v", err)
	}
	if _, err := a.Login("ip", "a long enough password", rec[0]); err == nil {
		t.Fatal("a recovery code must be single use")
	}
}

func TestBackoffAndNoCodeNoEntry(t *testing.T) {
	a, now := newAuth(t)
	res, _ := a.Setup(a.SetupKey(), "a long enough password")
	code, _ := totpAt(res.TOTPSecret, *now, 6)
	if _, err := a.Confirm("a long enough password", code); err != nil {
		t.Fatal(err)
	}
	// right password but no valid code must not get in
	if _, err := a.Login("ip", "a long enough password", "123456"); err == nil {
		t.Fatal("password alone must not be enough")
	}
	for i := 0; i < 3; i++ {
		_, _ = a.Login("ip", "bad", "000000")
	}
	good, _ := totpAt(res.TOTPSecret, *now, 6)
	if _, err := a.Login("ip", "a long enough password", good); err != ErrThrottled {
		t.Fatalf("expected throttling after repeated failures, got %v", err)
	}
	*now = now.Add(20 * time.Minute)
	good, _ = totpAt(res.TOTPSecret, *now, 6)
	if _, err := a.Login("ip", "a long enough password", good); err != nil {
		t.Fatalf("backoff must expire: %v", err)
	}
}

func TestSecretsNeverPrinted(t *testing.T) {
	s := secret.String("hunter2")
	if strings.Contains(s.String(), "hunter2") {
		t.Fatal("secret leaked through String()")
	}
}

func TestSessionSurvivesRestartAndStoresOnlyHashes(t *testing.T) {
	secret.SetKeyForTest(make([]byte, 32))
	dir := t.TempDir()
	a, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.Setup(a.SetupKey(), "a long enough password")
	if err != nil {
		t.Fatal(err)
	}
	code, _ := totpAt(res.TOTPSecret, time.Now(), 6)
	if _, err := a.Confirm("a long enough password", code); err != nil {
		t.Fatal(err)
	}
	a.Now = func() time.Time { return time.Now().Add(31 * time.Second) }
	code, _ = totpAt(res.TOTPSecret, a.Now(), 6)
	tok, err := a.Login("ip", "a long enough password", code)
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	b, err := New(dir) // a restart
	if err != nil {
		t.Fatal(err)
	}
	if !b.Valid(tok) {
		t.Fatal("a restart must not sign the admin out")
	}
	raw, _ := os.ReadFile(dir + "/sessions.json")
	if strings.Contains(string(raw), tok) {
		t.Fatal("the session file must hold hashes, never the token")
	}
	b.Logout(tok)
	c, _ := New(dir)
	if c.Valid(tok) {
		t.Fatal("a signed-out session must stay signed out after a restart")
	}
}
