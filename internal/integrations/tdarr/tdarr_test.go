package tdarr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v2/status" {
				t.Fatalf(
					"path = %q, want /api/v2/status",
					r.URL.Path,
				)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"status":"good",
				"isProduction":true,
				"os":"linux",
				"version":"2.89.01",
				"buildDate":"2026_09_19T11_07_51z",
				"serverEngine":"rust"
			}`))
		},
	))
	defer server.Close()

	result, err := New().TestConnection(
		context.Background(),
		server.URL+"/",
		"",
	)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	if !result.OK {
		t.Fatalf(
			"OK = false, message = %q",
			result.Message,
		)
	}

	if result.Version != "2.89.01" {
		t.Fatalf(
			"Version = %q, want 2.89.01",
			result.Version,
		)
	}
}

func TestConnectionRejectsBadStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"status":"bad",
				"version":"2.89.01"
			}`))
		},
	))
	defer server.Close()

	result, err := New().TestConnection(
		context.Background(),
		server.URL,
		"",
	)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	if result.OK {
		t.Fatal("OK = true, want false")
	}
}
