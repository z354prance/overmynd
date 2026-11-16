package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An arr's own poster loads only through a signed link, and carries the arr key server-side.
func TestArrCoverIsSignedAndKeyed(t *testing.T) {
	arr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "TOP-SECRET-ARR-KEY" || r.URL.Path != "/MediaCover/5/poster.jpg" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("\x89PNG\r\n\x1a\n0000000000000000"))
	}))
	defer arr.Close()
	e := newEnv(t)
	s := e.cfg.Get()
	s.Categories[0].ArrURL = arr.URL
	_ = e.cfg.Set(s)
	q := "/api/v1/playback/poster?arr=radarr&path=%2FMediaCover%2F5%2Fposter.jpg&signature="
	if code, _ := e.do("GET", q+"forged", nil, false); code != 404 {
		t.Fatalf("forged cover = %d", code)
	}
	if strings.Contains(q, "TOP-SECRET") {
		t.Fatal("key in url")
	}
}
