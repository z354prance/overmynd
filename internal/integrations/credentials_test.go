package integrations

import "testing"

func TestUsernamePasswordCredentialRoundTrip(t *testing.T) {
	encoded, err := EncodeUsernamePassword(
		"admin",
		"secret password",
	)
	if err != nil {
		t.Fatalf("EncodeUsernamePassword: %v", err)
	}

	decoded, err := DecodeUsernamePassword(encoded)
	if err != nil {
		t.Fatalf("DecodeUsernamePassword: %v", err)
	}

	if decoded.Username != "admin" {
		t.Fatalf(
			"Username = %q, want admin",
			decoded.Username,
		)
	}

	if decoded.Password != "secret password" {
		t.Fatalf(
			"Password = %q, want secret password",
			decoded.Password,
		)
	}
}

func TestDecodeUsernamePasswordRejectsInvalidJSON(t *testing.T) {
	_, err := DecodeUsernamePassword("not-json")
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
}

func TestDecodeUsernamePasswordRequiresUsername(t *testing.T) {
	_, err := DecodeUsernamePassword(
		`{"username":"","password":"secret"}`,
	)
	if err == nil {
		t.Fatal("expected username error")
	}
}
