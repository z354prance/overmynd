package qbittorrent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginAccepts204EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/v2/auth/login" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}

			if err := r.ParseForm(); err != nil {
				t.Fatalf("ParseForm: %v", err)
			}

			if r.Form.Get("username") != "testuser" {
				t.Fatalf(
					"username = %q, want testuser",
					r.Form.Get("username"),
				)
			}

			if r.Form.Get("password") != "testpass" {
				t.Fatalf("unexpected password")
			}

			http.SetCookie(w, &http.Cookie{
				Name:  "QBT_SID_8080",
				Value: "test-session",
				Path:  "/",
			})

			w.WriteHeader(http.StatusNoContent)
		},
	))
	defer server.Close()

	client := NewClient()

	err := client.Login(
		context.Background(),
		server.URL,
		"testuser",
		"testpass",
	)
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
}
