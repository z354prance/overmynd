package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/z354prance/overmynd/internal/auth"
	"github.com/z354prance/overmynd/internal/database"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/services"
)

func TestAuthLifecycleAPI(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	authManager := auth.NewManager(db)
	handler := NewRouter(
		services.NewManager(db),
		integrations.NewRegistry(),
		authManager,
	)

	status := request(t, handler, "", http.MethodGet, "/api/v1/auth/status", "")
	if status.Code != http.StatusOK {
		t.Fatalf("initial auth status = %d: %s", status.Code, status.Body.String())
	}

	var initial struct {
		SetupRequired bool `json:"setup_required"`
		Authenticated bool `json:"authenticated"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &initial); err != nil {
		t.Fatalf("decode initial auth status: %v", err)
	}
	if !initial.SetupRequired || initial.Authenticated {
		t.Fatalf("unexpected initial auth status: %+v", initial)
	}

	setup := request(
		t,
		handler,
		"",
		http.MethodPost,
		"/api/v1/auth/setup",
		"{"username":"admin","password":"correct horse battery staple"}",
	)
	if setup.Code != http.StatusCreated {
		t.Fatalf("setup = %d: %s", setup.Code, setup.Body.String())
	}

	cookies := setup.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || cookies[0].Value == "" {
		t.Fatalf("setup did not issue session cookie: %+v", cookies)
	}

	token := cookies[0].Value

	status = request(t, handler, token, http.MethodGet, "/api/v1/auth/status", "")
	if status.Code != http.StatusOK {
		t.Fatalf("authenticated auth status = %d: %s", status.Code, status.Body.String())
	}

	var authenticated struct {
		SetupRequired bool   `json:"setup_required"`
		Authenticated bool   `json:"authenticated"`
		Username      string `json:"username"`
	}
	if err := json.Unmarshal(status.Body.Bytes(), &authenticated); err != nil {
		t.Fatalf("decode authenticated auth status: %v", err)
	}
	if authenticated.SetupRequired || !authenticated.Authenticated || authenticated.Username != "admin" {
		t.Fatalf("unexpected authenticated status: %+v", authenticated)
	}

	setupAgain := request(
		t,
		handler,
		"",
		http.MethodPost,
		"/api/v1/auth/setup",
		"{"username":"another","password":"another secure password"}",
	)
	if setupAgain.Code != http.StatusConflict {
		t.Fatalf("second setup = %d, want %d: %s", setupAgain.Code, http.StatusConflict, setupAgain.Body.String())
	}

	logout := request(t, handler, token, http.MethodPost, "/api/v1/auth/logout", "")
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout = %d: %s", logout.Code, logout.Body.String())
	}

	login := request(
		t,
		handler,
		"",
		http.MethodPost,
		"/api/v1/auth/login",
		"{"username":"admin","password":"correct horse battery staple"}",
	)
	if login.Code != http.StatusOK {
		t.Fatalf("login = %d: %s", login.Code, login.Body.String())
	}

	loginCookies := login.Result().Cookies()
	if len(loginCookies) != 1 || loginCookies[0].Name != sessionCookieName || loginCookies[0].Value == "" {
		t.Fatalf("login did not issue session cookie: %+v", loginCookies)
	}

	wrong := request(
		t,
		handler,
		"",
		http.MethodPost,
		"/api/v1/auth/login",
		"{"username":"admin","password":"wrong password"}",
	)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password = %d, want %d", wrong.Code, http.StatusUnauthorized)
	}
}
