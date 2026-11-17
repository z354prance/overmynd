package seerr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v1/status" {
				t.Fatalf(
					"path = %q, want /api/v1/status",
					r.URL.Path,
				)
			}

			if r.Header.Get("X-Api-Key") != "test-api-key" {
				t.Fatal("missing or incorrect X-Api-Key")
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"version":"2.7.3"}`,
			))
		},
	))
	defer server.Close()

	result, err := New().TestConnection(
		context.Background(),
		server.URL,
		"test-api-key",
	)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	if !result.OK {
		t.Fatal("connection result should be OK")
	}

	if result.Version != "2.7.3" {
		t.Fatalf(
			"version = %q, want 2.7.3",
			result.Version,
		)
	}
}

func TestConnectionRequiresAPIKey(t *testing.T) {
	result, err := New().TestConnection(
		context.Background(),
		"http://example.invalid",
		"",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.OK {
		t.Fatal("connection should not be OK without API key")
	}
}
