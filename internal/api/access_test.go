package api

import (
	"strings"
	"testing"
)

func TestAccessAdminBoundary(t *testing.T) {
	h, token := testRouter(t)
	for _, route := range []struct{ method, path string }{{"GET", "settings"}, {"PUT", "settings"}, {"GET", "requests"}, {"POST", "test-email"}, {"POST", "requests/1/approve"}, {"POST", "requests/1/decline"}, {"POST", "requests/1/resend"}, {"POST", "requests/1/correct"}} {
		w := request(t, h, "", route.method, "/api/v1/access/"+route.path, `{}`)
		if w.Code != 401 {
			t.Fatalf("public %s: %d %s", route.path, w.Code, w.Body)
		}
	}
	w := request(t, h, "", "GET", "/api/v1/access/status", "")
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != `{"enabled":false}` {
		t.Fatal(w.Code, w.Body)
	}
	w = request(t, h, "", "POST", "/api/v1/access/request", `{"name":"Alice","email":"alice@example.com","username":"alice","connect_username":"alice-connect"}`)
	if w.Code != 400 {
		t.Fatal("disabled accepted", w.Code)
	}
	settings := `{"enabled":false,"emby_key":"private-emby-key","smtp_password":"private-mail-password"}`
	w = request(t, h, token, "PUT", "/api/v1/access/settings", settings)
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-") {
		t.Fatal(w.Code, w.Body)
	}
	w = request(t, h, token, "GET", "/api/v1/access/settings", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-") || !strings.Contains(w.Body.String(), `"has_emby_key":true`) {
		t.Fatal(w.Code, w.Body)
	}
	w = request(t, h, token, "GET", "/api/v1/access/requests", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"requests":[]`) {
		t.Fatal(w.Code, w.Body)
	}
	w = request(t, h, "", "POST", "/api/v1/access/setup", `{"token":"invalid","password":"a long test password"}`)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
}
func TestAccessRejectsExtraFieldsAndThrottles(t *testing.T) {
	h, _ := testRouter(t)
	for i := 0; i < 10; i++ {
		w := request(t, h, "", "POST", "/api/v1/access/request", `{"status":"active"}`)
		if w.Code != 400 {
			t.Fatal(w.Code, w.Body)
		}
	}
	w := request(t, h, "", "POST", "/api/v1/access/request", `{}`)
	if w.Code != 429 {
		t.Fatal("not rate limited", w.Code)
	}
}
