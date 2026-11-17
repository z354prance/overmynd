package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestDecisions(t *testing.T) {
	writes, status, failure := 0, 1, false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "private-key" || r.Header.Get("X-API-User") != "" {
			t.Error("incorrect approval identity")
		}
		if r.Method == "GET" {
			fmt.Fprintf(w, `{"status":%d}`, status)
			return
		}
		writes++
		if r.URL.Path != "/api/v1/request/42/approve" && r.URL.Path != "/api/v1/request/42/decline" {
			t.Error(r.URL.Path)
		}
		if failure {
			http.Error(w, "private-key", 500)
			return
		}
		fmt.Fprint(w, `{"status":2}`)
	}))
	defer upstream.Close()
	handler, token := testRouter(t)
	created := request(t, handler, token, "POST", "/api/v1/services", fmt.Sprintf(`{"type":"seerr","name":"Seerr","enabled":true,"base_url":%q,"credential":"private-key"}`, upstream.URL))
	var service struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &service); err != nil || service.ID == 0 {
		t.Fatal(created.Body.String())
	}
	route := fmt.Sprintf("/api/v1/requests/%d/42/", service.ID)
	if got := request(t, handler, "", "POST", route+"approve", ""); got.Code != 401 {
		t.Fatal(got.Code)
	}
	for _, origin := range []string{"", "https://evil.example"} {
		req := httptest.NewRequest("POST", route+"approve", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
		if origin != "" {
			req.Header.Set("Origin", origin)
			req.Header.Set("X-Overmynd-Request", "1")
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		if result.Code != 403 {
			t.Fatalf("unsafe mutation: %d", result.Code)
		}
	}
	if writes != 0 {
		t.Fatal("unauthorized write")
	}
	for _, action := range []string{"approve", "decline"} {
		if got := request(t, handler, token, "POST", route+action, ""); got.Code != 200 {
			t.Fatal(got.Code, got.Body.String())
		}
	}
	status = 2
	if got := request(t, handler, token, "POST", route+"approve", ""); got.Code != 409 {
		t.Fatal(got.Code)
	}
	if writes != 2 {
		t.Fatal("stale decision mutated request")
	}
	status = 1
	failure = true
	got := request(t, handler, token, "POST", route+"approve", "")
	if got.Code != 502 || strings.Contains(got.Body.String(), "private-key") || writes != 3 {
		t.Fatal("error disclosure or retry", got.Code, writes)
	}
	if got := request(t, handler, token, "POST", route+"delete", ""); got.Code != 400 {
		t.Fatal(got.Code)
	}
}
