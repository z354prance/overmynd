package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicRequestsUseConfiguredUser(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "private-key" || r.Header.Get("X-API-User") != "7" {
			t.Errorf("wrong identity headers: %v", r.Header)
			w.WriteHeader(403)
			return
		}
		switch r.URL.Path {
		case "/api/v1/auth/me":
			fmt.Fprint(w, `{"id":7}`)
		case "/api/v1/search":
			if r.URL.Query().Get("query") != "test & title" {
				t.Errorf("query not encoded correctly: %s", r.URL.RawQuery)
			}
			fmt.Fprint(w, `{"results":[{"id":1,"mediaType":"movie","title":"Test","secret":"private-key"},{"id":2,"mediaType":"person","name":"Person"}]}`)
		case "/api/v1/request":
			requests++
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			for _, forbidden := range []string{"userId", "ignoreQuota", "is4k", "serverId", "profileId"} {
				if _, exists := payload[forbidden]; exists {
					t.Errorf("unexpected override: %s", forbidden)
				}
			}
			if payload["mediaType"] == "tv" && payload["seasons"] != "all" {
				t.Errorf("wrong TV seasons: %v", payload)
			}
			w.WriteHeader(201)
			fmt.Fprint(w, `{"id":123,"status":1,"requestedBy":{"email":"private@example.com"}}`)
		default:
			t.Errorf("unexpected upstream route: %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	handler, token := testRouter(t)
	create := request(t, handler, token, "POST", "/api/v1/services", fmt.Sprintf(`{"type":"seerr","name":"Seerr","enabled":true,"base_url":%q,"credential":"private-key"}`, upstream.URL))
	if create.Code != 201 {
		t.Fatal(create.Body.String())
	}
	var service struct {
		ID int64 `json:"id"`
	}
	json.Unmarshal(create.Body.Bytes(), &service)
	config := fmt.Sprintf(`{"enabled":true,"service_id":%d,"user_id":7}`, service.ID)
	for _, method := range []string{"GET", "PUT"} {
		w := request(t, handler, "", method, "/api/v1/request-settings", config)
		if w.Code != 401 {
			t.Fatalf("anonymous configuration: %d", w.Code)
		}
	}
	w := request(t, handler, "", "POST", "/api/v1/media-request", `{"media_id":1,"media_type":"movie"}`)
	if w.Code != 503 {
		t.Fatalf("requests should default off: %d", w.Code)
	}
	w = request(t, handler, token, "PUT", "/api/v1/request-settings", config)
	if w.Code != 200 {
		t.Fatalf("configure: %d %s", w.Code, w.Body)
	}
	w = request(t, handler, "", "GET", "/api/v1/media-request/status", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "user_id") || !strings.Contains(w.Body.String(), "true") {
		t.Fatal(w.Body.String())
	}
	w = request(t, handler, "", "GET", "/api/v1/media-request/search?query=test+%26+title", "")
	if w.Code != 200 || strings.Contains(w.Body.String(), "private-key") || strings.Contains(w.Body.String(), "Person") {
		t.Fatalf("search: %d %s", w.Code, w.Body)
	}
	for _, body := range []string{`{"media_id":1,"media_type":"movie","user_id":1}`, `{"media_id":1,"media_type":"movie","ignoreQuota":true}`, `{"media_id":1,"media_type":"tv"}`} {
		if w := request(t, handler, "", "POST", "/api/v1/media-request", body); w.Code != 400 {
			t.Fatalf("invalid payload: %d", w.Code)
		}
	}
	if requests != 0 {
		t.Fatal("invalid request reached Seerr")
	}
	r := httptest.NewRequest("POST", "http://example.com/api/v1/media-request", strings.NewReader(`{"media_id":1,"media_type":"movie"}`))
	r.Header.Set("Origin", "https://attacker.example")
	r.Header.Set("X-Overmynd-Request", "1")
	rejected := httptest.NewRecorder()
	handler.ServeHTTP(rejected, r)
	if rejected.Code != 403 || requests != 0 {
		t.Fatal("cross-origin request was not blocked")
	}
	for i := 0; i < 10; i++ {
		w = request(t, handler, "", "POST", "/api/v1/media-request", `{"media_id":1,"media_type":"tv","all_seasons":true}`)
		if w.Code != 201 || strings.Contains(w.Body.String(), "email") || !strings.Contains(w.Body.String(), `"status":1`) {
			t.Fatalf("public request: %d %s", w.Code, w.Body)
		}
	}
	if w := request(t, handler, "", "POST", "/api/v1/media-request", `{"media_id":1,"media_type":"movie"}`); w.Code != 429 {
		t.Fatalf("missing rate limit: %d", w.Code)
	}
}

func TestRequestSettingsRejectWrongSeerrIdentity(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"id":1}`) }))
	defer upstream.Close()
	handler, token := testRouter(t)
	w := request(t, handler, token, "POST", "/api/v1/services", fmt.Sprintf(`{"type":"seerr","name":"Seerr","enabled":true,"base_url":%q,"credential":"key"}`, upstream.URL))
	var service struct{ ID int64 }
	json.Unmarshal(w.Body.Bytes(), &service)
	w = request(t, handler, token, "PUT", "/api/v1/request-settings", fmt.Sprintf(`{"enabled":true,"service_id":%d,"user_id":7}`, service.ID))
	if w.Code != 400 {
		t.Fatalf("wrong Seerr identity accepted: %d", w.Code)
	}
}
