package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/z354prance/overmynd/internal/integrations"
)

func TestConnection(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			switch r.URL.Path {
			case "/api/v2/auth/login":
				if r.Method != http.MethodPost {
					t.Fatalf(
						"method = %q, want POST",
						r.Method,
					)
				}

				if err := r.ParseForm(); err != nil {
					t.Fatalf("ParseForm: %v", err)
				}

				if r.Form.Get("username") != "admin" {
					t.Fatalf("wrong username")
				}

				if r.Form.Get("password") != "secret" {
					t.Fatalf("wrong password")
				}

				http.SetCookie(
					w,
					&http.Cookie{
						Name:  "SID",
						Value: "test-session",
						Path:  "/",
					},
				)

				_, _ = w.Write([]byte("Ok."))

			case "/api/v2/app/version":
				cookie, err := r.Cookie("SID")
				if err != nil {
					t.Fatalf("SID cookie missing: %v", err)
				}

				if cookie.Value != "test-session" {
					t.Fatalf(
						"SID = %q",
						cookie.Value,
					)
				}

				_, _ = w.Write([]byte("v5.1.2"))

			default:
				http.NotFound(w, r)
			}
		}),
	)
	defer server.Close()

	credential, err := integrations.EncodeUsernamePassword(
		"admin",
		"secret",
	)
	if err != nil {
		t.Fatalf("encode credential: %v", err)
	}

	integration := New()

	result, err := integration.TestConnection(
		context.Background(),
		server.URL,
		credential,
	)
	if err != nil {
		t.Fatalf("TestConnection: %v", err)
	}

	if !result.OK {
		t.Fatal("expected successful connection")
	}

	if result.Version != "v5.1.2" {
		t.Fatalf(
			"Version = %q, want v5.1.2",
			result.Version,
		)
	}
}
