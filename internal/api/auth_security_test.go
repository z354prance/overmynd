package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminWriteRejectsCrossOrigin(t *testing.T) {
	handler, sessionToken := testRouter(t)
	req := httptest.NewRequest(http.MethodPost, "http://overmynd.local/api/v1/services", bytes.NewBufferString(`{"type":"radarr","name":"Radarr","enabled":true,"base_url":"http://10.0.0.10:7878","credential":"test-key"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://attacker.example")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-origin admin write = %d, want %d: %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

func TestAdminWriteAcceptsSameOrigin(t *testing.T) {
	handler, sessionToken := testRouter(t)
	req := httptest.NewRequest(http.MethodPost, "http://overmynd.local/api/v1/services", bytes.NewBufferString(`{"type":"radarr","name":"Radarr","enabled":true,"base_url":"http://10.0.0.10:7878","credential":"test-key"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://overmynd.local")
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionToken})
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("same-origin admin write = %d, want %d: %s", recorder.Code, http.StatusCreated, recorder.Body.String())
	}
}

func TestSameOriginBehindHTTPSProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://overmynd:8080/api/v1/services", nil)
	req.Host = "overmynd.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Origin", "https://overmynd.example.com")
	if !sameOriginRequest(req) {
		t.Fatal("expected HTTPS reverse-proxy origin to be accepted")
	}
	req.Header.Set("Origin", "https://attacker.example")
	if sameOriginRequest(req) {
		t.Fatal("expected cross-origin proxy request to be rejected")
	}
}
