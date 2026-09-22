package nzbget

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/integrations"
)

func TestConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/jsonrpc" {
				t.Fatalf("path = %q, want /jsonrpc", r.URL.Path)
			}

			username, password, ok := r.BasicAuth()
			if !ok {
				t.Fatal("missing basic auth")
			}
			if username != "nzbuser" {
				t.Fatalf("username = %q, want nzbuser", username)
			}
			if password != "nzbpass" {
				t.Fatal("unexpected password")
			}

			var request rpcRequest
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatalf("decode request: %v", err)
			}

			if request.Method != "version" {
				t.Fatalf(
					"method = %q, want version",
					request.Method,
				)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"version":"1.1","result":"24.8","error":null,"id":1}`,
			))
		},
	))
	defer server.Close()

	credential, err := integrations.EncodeUsernamePassword(
		"nzbuser",
		"nzbpass",
	)
	if err != nil {
		t.Fatalf("encode credentials: %v", err)
	}

	result, err := New().TestConnection(
		context.Background(),
		server.URL,
		credential,
	)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	if !result.OK {
		t.Fatal("connection result should be OK")
	}
	if result.Version != "24.8" {
		t.Fatalf(
			"version = %q, want 24.8",
			result.Version,
		)
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(
				`{"version":"1.1","result":null,"error":{"code":1,"message":"test error"},"id":1}`,
			))
		},
	))
	defer server.Close()

	client := NewClient()

	var version string
	err := client.Call(
		context.Background(),
		server.URL,
		"user",
		"pass",
		"version",
		&version,
	)

	if err == nil {
		t.Fatal("expected RPC error")
	}
}
