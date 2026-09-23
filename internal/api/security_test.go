package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteOriginProtection(t *testing.T) {
	handler, token := testRouter(t)
	routes := []struct{ method, path string }{
		{"POST", "/api/v1/auth/setup"}, {"POST", "/api/v1/auth/login"},
		{"POST", "/api/v1/auth/logout"}, {"POST", "/api/v1/services"},
		{"PUT", "/api/v1/services/1"}, {"DELETE", "/api/v1/services/1"},
		{"POST", "/api/v1/services/1/test"},
	}
	for _, route := range routes {
		for _, attack := range []struct{ name, origin, site, header string }{
			{"form", "", "", ""},
			{"cross-origin", "https://evil.example", "", "1"},
			{"null-origin", "null", "", "1"},
			{"cross-site", "", "cross-site", "1"},
			{"sibling-origin", "http://sibling.example.com", "same-site", "1"},
		} {
			t.Run(route.method+route.path+attack.name, func(t *testing.T) {
				r := httptest.NewRequest(route.method, "http://example.com"+route.path, strings.NewReader(`{}`))
				r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
				r.Header.Set("Origin", attack.origin)
				r.Header.Set("Sec-Fetch-Site", attack.site)
				r.Header.Set("X-Overmynd-Request", attack.header)
				r.Header.Set("X-Forwarded-Host", "evil.example")
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code != http.StatusForbidden {
					t.Fatalf("got %d: %s", w.Code, w.Body)
				}
			})
		}
	}
	// Rejected logout requests must not invalidate the session.
	if w := request(t, handler, token, "GET", "/api/v1/services", ""); w.Code != 200 {
		t.Fatal(w.Code)
	}
}

func TestOriginProtectionAllowsSameOriginAndCLI(t *testing.T) {
	for _, headers := range []map[string]string{
		{}, {"Origin": "http://example.com"}, {"Sec-Fetch-Site": "same-origin"},
		{"Origin": "https://example.com", "X-Forwarded-Proto": "https"},
	} {
		called := false
		handler := protectRequests(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
		r := httptest.NewRequest("POST", "http://example.com/api/v1/services", nil)
		r.Header.Set("X-Overmynd-Request", "1")
		for key, value := range headers {
			r.Header.Set(key, value)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if !called {
			t.Fatalf("legitimate request rejected: %v: %s", headers, w.Body)
		}
	}
}

func TestPublicReadsRemainPublic(t *testing.T) {
	handler, _ := testRouter(t)
	for _, path := range []string{"/health", "/api/v1/status", "/api/v1/public/services", "/api/v1/activity", "/api/v1/downloads", "/api/v1/processing", "/api/v1/playback", "/api/v1/missing", "/api/v1/requests"} {
		r := httptest.NewRequest("GET", path, nil)
		r.Header.Set("Sec-Fetch-Site", "cross-site")
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body)
		}
	}
}

func TestAuthResponseNotCachedAndCookieFlags(t *testing.T) {
	handler, _ := testRouter(t)
	r := httptest.NewRequest("POST", "https://example.com/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"correct horse battery staple"}`))
	r.Header.Set("X-Overmynd-Request", "1")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("%d %v", w.Code, w.Header())
	}
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookies: %+v", cookies)
	}
}

func TestRejectOversizedAndTrailingJSON(t *testing.T) {
	handler, _ := testRouter(t)
	for _, body := range []string{`{"username":"admin","password":"password"} {}`, `{"username":"admin","password":"` + strings.Repeat("x", 65536) + `"}`} {
		w := request(t, handler, "", "POST", "/api/v1/auth/login", body)
		if w.Code != 400 {
			t.Fatalf("got %d", w.Code)
		}
	}
}
