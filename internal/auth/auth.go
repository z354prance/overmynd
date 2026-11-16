// Package auth is the admin account: password, required TOTP, recovery codes,
// sessions, and login backoff. Standard library only.
package auth

import (
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/undermynd/internal/secret"
)

const pbkdf2Iterations = 600_000

var (
	ErrNoAccount   = errors.New("no admin account yet")
	ErrBadLogin    = errors.New("invalid credentials")
	ErrThrottled   = errors.New("too many attempts, try again later")
	ErrBadSetupKey = errors.New("invalid setup key")
	ErrWeak        = errors.New("password must be at least 12 characters")
	ErrExists      = errors.New("an admin account already exists")
)

type account struct {
	PasswordHash string        `json:"passwordHash"`
	TOTPSecret   secret.String `json:"totpSecret"`
	TOTPActive   bool          `json:"totpActive"`
	Recovery     []string      `json:"recovery"` // sha256 hex of unused codes
	Created      time.Time     `json:"created"`
}

type Auth struct {
	mu       sync.Mutex
	path     string
	spath    string // where sign-in sessions are kept so a restart does not sign you out
	acct     *account
	setupKey string
	sessions map[string]time.Time // sha256(token) -> expiry
	fails    map[string]*failure
	Now      func() time.Time
	// SessionTTL is how long a sign-in lasts.
	SessionTTL time.Duration
}

type failure struct {
	n     int
	until time.Time
}

// New loads the account if it exists. With no account it generates a one-time
// setup key (kept on disk and printed to the log) that first-run setup requires,
// so nobody who merely reaches the page can claim the admin account.
func New(configDir string) (*Auth, error) {
	a := &Auth{path: configDir + "/admin.json", spath: configDir + "/sessions.json", sessions: map[string]time.Time{}, fails: map[string]*failure{},
		Now: time.Now, SessionTTL: 30 * 24 * time.Hour}
	a.loadSessions()
	b, err := os.ReadFile(a.path)
	switch {
	case err == nil:
		var ac account
		if err := json.Unmarshal(b, &ac); err != nil {
			return nil, err
		}
		a.acct = &ac
	case errors.Is(err, os.ErrNotExist):
		keyPath := configDir + "/setup-key"
		kb, kerr := os.ReadFile(keyPath)
		if kerr != nil {
			kb = []byte(randomString(20))
			if err := os.WriteFile(keyPath, kb, 0o600); err != nil {
				return nil, err
			}
		}
		a.setupKey = strings.TrimSpace(string(kb))
	default:
		return nil, err
	}
	return a, nil
}

// SetupKey is non-empty only while no account exists.
func (a *Auth) SetupKey() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acct != nil {
		return ""
	}
	return a.setupKey
}

func (a *Auth) HasAccount() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.acct != nil && a.acct.TOTPActive
}

// Pending reports that setup started but the authenticator was never confirmed.
func (a *Auth) Pending() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.acct != nil && !a.acct.TOTPActive
}

func (a *Auth) saveLocked() error {
	b, err := json.MarshalIndent(a.acct, "", "  ")
	if err != nil {
		return err
	}
	tmp := a.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.path)
}

type SetupResult struct {
	TOTPSecret string `json:"totpSecret"`
	OTPAuthURI string `json:"otpauthUri"`
}

// Setup creates the one admin account. It needs the setup key and succeeds only
// once, even if called twice at the same moment. The account is not usable
// until the authenticator is confirmed with Confirm.
func (a *Auth) Setup(key, password string) (SetupResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acct != nil {
		return SetupResult{}, ErrExists
	}
	if subtle.ConstantTimeCompare([]byte(key), []byte(a.setupKey)) != 1 {
		return SetupResult{}, ErrBadSetupKey
	}
	if len(password) < 12 {
		return SetupResult{}, ErrWeak
	}
	h, err := HashPassword(password)
	if err != nil {
		return SetupResult{}, err
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return SetupResult{}, err
	}
	sec := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	a.acct = &account{PasswordHash: h, TOTPSecret: secret.String(sec), Created: a.Now()}
	if err := a.saveLocked(); err != nil {
		a.acct = nil
		return SetupResult{}, err
	}
	u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/Undermynd:admin"}
	q := url.Values{"secret": {sec}, "issuer": {"Undermynd"}}
	u.RawQuery = q.Encode()
	return SetupResult{TOTPSecret: sec, OTPAuthURI: u.String()}, nil
}

// Confirm activates the account after the user proves the authenticator works,
// and returns the one-time recovery codes.
func (a *Auth) Confirm(password, code string) ([]string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acct == nil || a.acct.TOTPActive {
		return nil, ErrExists
	}
	if !VerifyPassword(password, a.acct.PasswordHash) || !totpValid(string(a.acct.TOTPSecret), code, a.Now()) {
		return nil, ErrBadLogin
	}
	codes := make([]string, 8)
	a.acct.Recovery = nil
	for i := range codes {
		codes[i] = randomString(10)
		a.acct.Recovery = append(a.acct.Recovery, hashCode(codes[i]))
	}
	a.acct.TOTPActive = true
	if err := a.saveLocked(); err != nil {
		a.acct.TOTPActive = false
		return nil, err
	}
	return codes, nil
}

// Login checks password plus a TOTP (or recovery) code and returns a session token.
func (a *Auth) Login(client, password, code string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := a.Now()
	if f := a.fails[client]; f != nil && now.Before(f.until) {
		return "", ErrThrottled
	}
	if a.acct == nil || !a.acct.TOTPActive {
		return "", ErrNoAccount
	}
	passOK := VerifyPassword(password, a.acct.PasswordHash)
	codeOK := totpValid(string(a.acct.TOTPSecret), code, now)
	if !codeOK {
		h := hashCode(strings.TrimSpace(code))
		for i, r := range a.acct.Recovery {
			if subtle.ConstantTimeCompare([]byte(r), []byte(h)) == 1 && passOK {
				a.acct.Recovery = append(a.acct.Recovery[:i], a.acct.Recovery[i+1:]...)
				_ = a.saveLocked()
				codeOK = true
				break
			}
		}
	}
	if !(passOK && codeOK) {
		f := a.fails[client]
		if f == nil {
			f = &failure{}
			a.fails[client] = f
		}
		f.n++
		if f.n >= 3 { // back off: 2s, 4s, 8s ... capped at 15 minutes
			d := time.Duration(1<<uint(min(f.n-2, 9))) * time.Second
			if d > 15*time.Minute {
				d = 15 * time.Minute
			}
			f.until = now.Add(d)
		}
		return "", ErrBadLogin
	}
	delete(a.fails, client)
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := base64.RawURLEncoding.EncodeToString(raw)
	a.sessions[hashCode(tok)] = now.Add(a.SessionTTL)
	a.saveSessionsLocked()
	return tok, nil
}

func (a *Auth) Valid(token string) bool {
	if token == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	h := hashCode(token)
	exp, ok := a.sessions[h]
	if !ok {
		return false
	}
	if a.Now().After(exp) {
		delete(a.sessions, h)
		return false
	}
	return true
}

func (a *Auth) Logout(token string) {
	a.mu.Lock()
	delete(a.sessions, hashCode(token))
	a.saveSessionsLocked()
	a.mu.Unlock()
}

// ---------------------------------------------------------------- crypto

func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	k, err := pbkdf2.Key(sha256.New, pw, salt, pbkdf2Iterations, 32)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", pbkdf2Iterations,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(k)), nil
}

func VerifyPassword(pw, enc string) bool {
	parts := strings.Split(enc, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	var iter int
	if _, err := fmt.Sscanf(parts[1], "%d", &iter); err != nil || iter < 100_000 {
		return false
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[2])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got, err := pbkdf2.Key(sha256.New, pw, salt, iter, len(want))
	return err == nil && subtle.ConstantTimeCompare(got, want) == 1
}

func hashCode(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func randomString(n int) string {
	const alpha = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, n)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alpha[int(b[i])%len(alpha)]
	}
	return string(b)
}

// TOTP per RFC 6238 (SHA-1, 30 s, 6 digits), accepting one step either side.
func totpAt(secretB32 string, t time.Time, digits int) (string, bool) {
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secretB32))
	if err != nil {
		return "", false
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(t.Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := (uint32(sum[off]&0x7f) << 24) | uint32(sum[off+1])<<16 | uint32(sum[off+2])<<8 | uint32(sum[off+3])
	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, code%mod), true
}

func totpValid(secretB32, code string, now time.Time) bool {
	code = strings.TrimSpace(strings.ReplaceAll(code, " ", ""))
	if len(code) != 6 {
		return false
	}
	ok := false
	for _, d := range []time.Duration{-30 * time.Second, 0, 30 * time.Second} {
		if want, good := totpAt(secretB32, now.Add(d), 6); good && subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			ok = true // keep checking all steps so timing does not reveal which matched
		}
	}
	return ok
}

// CodeAt returns the current 6-digit code for a base32 secret (used by tests and tooling).
func CodeAt(secretB32 string, t time.Time) string {
	c, _ := totpAt(secretB32, t, 6)
	return c
}

// Sessions are kept as hashes only, so the file cannot be used to sign in.
func (a *Auth) loadSessions() {
	b, err := os.ReadFile(a.spath)
	if err != nil {
		return
	}
	var m map[string]time.Time
	if json.Unmarshal(b, &m) != nil {
		return
	}
	now := a.Now()
	for h, exp := range m {
		if exp.After(now) {
			a.sessions[h] = exp
		}
	}
}

func (a *Auth) saveSessionsLocked() {
	now := a.Now()
	for h, exp := range a.sessions {
		if !exp.After(now) {
			delete(a.sessions, h)
		}
	}
	b, err := json.Marshal(a.sessions)
	if err != nil {
		return
	}
	tmp := a.spath + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, a.spath)
	}
}
