package seerr

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserJSONFailsClosed(t *testing.T) {
	calls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(403)
		fmt.Fprint(w, "private-key secret-url")
	}))
	defer upstream.Close()
	if err := UserJSON(context.Background(), upstream.URL, "private-key", 0, "POST", "request", nil, nil); err == nil || calls != 0 {
		t.Fatal("missing user must fail without calling Seerr")
	}
	err := UserJSON(context.Background(), upstream.URL, "private-key", 7, "POST", "request", nil, nil)
	if err == nil || strings.Contains(err.Error(), "private-key") || strings.Contains(err.Error(), "secret-url") {
		t.Fatalf("unsafe upstream error: %v", err)
	}
}

func TestUserJSONDoesNotFollowRedirects(t *testing.T) {
	calls := 0
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer target.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer upstream.Close()
	err := UserJSON(context.Background(), upstream.URL, "private-key", 7, "POST", "request", nil, nil)
	if err == nil || calls != 0 {
		t.Fatal("redirect could leak API credentials")
	}
}
