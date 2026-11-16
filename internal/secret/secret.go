// Package secret encrypts stored secrets at rest with AES-256-GCM.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
)

const prefix = "enc:v1:"

var (
	mu  sync.RWMutex
	key []byte
)

// LoadKey reads the key file, creating it (mode 0600) on first run.
func LoadKey(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		b = make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return err
		}
		if err := os.WriteFile(path, b, 0o600); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if len(b) != 32 {
		return errors.New("secret key file must be 32 bytes")
	}
	mu.Lock()
	key = b
	mu.Unlock()
	return nil
}

func SetKeyForTest(b []byte) { mu.Lock(); key = b; mu.Unlock() }

func Encrypt(plain string) (string, error) {
	mu.RLock()
	k := key
	mu.RUnlock()
	if k == nil {
		return "", errors.New("secret key not loaded")
	}
	blk, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return prefix + base64.RawStdEncoding.EncodeToString(g.Seal(nonce, nonce, []byte(plain), nil)), nil
}

func Decrypt(s string) (string, error) {
	if !strings.HasPrefix(s, prefix) {
		return s, nil // plaintext written by hand; re-encrypted on next save
	}
	mu.RLock()
	k := key
	mu.RUnlock()
	if k == nil {
		return "", errors.New("secret key not loaded")
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil {
		return "", err
	}
	blk, err := aes.NewCipher(k)
	if err != nil {
		return "", err
	}
	g, err := cipher.NewGCM(blk)
	if err != nil {
		return "", err
	}
	if len(raw) < g.NonceSize() {
		return "", errors.New("short ciphertext")
	}
	pt, err := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
	return string(pt), err
}

// String is a secret value: plain in memory, encrypted in JSON, and never
// printed by fmt or logs.
type String string

func (s String) MarshalJSON() ([]byte, error) {
	if s == "" {
		return json.Marshal("")
	}
	e, err := Encrypt(string(s))
	if err != nil {
		return nil, err
	}
	return json.Marshal(e)
}

func (s *String) UnmarshalJSON(b []byte) error {
	var raw string
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if raw == "" {
		*s = ""
		return nil
	}
	p, err := Decrypt(raw)
	if err != nil {
		return err
	}
	*s = String(p)
	return nil
}

func (s String) String() string   { return "[secret]" }
func (s String) GoString() string { return "[secret]" }
