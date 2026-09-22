package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z354prance/overmynd/internal/auth"
	"github.com/z354prance/overmynd/internal/database"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/radarr"
	"github.com/z354prance/overmynd/internal/models"
	"github.com/z354prance/overmynd/internal/services"
)

func testRouter(t *testing.T) (http.Handler, string) {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}

	if err := db.Migrate(); err != nil {
		db.Close()
		t.Fatalf("migrate database: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	authManager := auth.NewManager(db)
	session, err := authManager.Setup(
		"admin",
		"correct horse battery staple",
	)
	if err != nil {
		t.Fatalf("setup administrator: %v", err)
	}

	handler := NewRouter(
		services.NewManager(db),
		integrations.NewRegistry(),
		authManager,
	)

	return handler, session.Token
}

func request(
	t *testing.T,
	handler http.Handler,
	sessionToken string,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody *bytes.Reader

	requestBody = bytes.NewReader([]byte(body))

	req := httptest.NewRequest(
		method,
		path,
		requestBody,
	)

	if body != "" {
		req.Header.Set(
			"Content-Type",
			"application/json",
		)
	}

	if sessionToken != "" {
		req.AddCookie(&http.Cookie{
			Name:  sessionCookieName,
			Value: sessionToken,
		})
	}

	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, req)

	return recorder
}

func TestServiceAPI(t *testing.T) {
	handler, sessionToken := testRouter(t)

	createBody := `{
		"type":"radarr",
		"name":"Radarr",
		"enabled":true,
		"base_url":"http://10.0.0.10:7878/",
		"credential":"super-secret-api-key"
	}`

	create := request(
		t,
		handler,
		sessionToken,
		http.MethodPost,
		"/api/v1/services",
		createBody,
	)

	if create.Code != http.StatusCreated {
		t.Fatalf(
			"create returned %d: %s",
			create.Code,
			create.Body.String(),
		)
	}

	if strings.Contains(
		create.Body.String(),
		"super-secret-api-key",
	) {
		t.Fatal("credential leaked in create response")
	}

	if strings.Contains(
		create.Body.String(),
		`"credential"`,
	) {
		t.Fatal("credential field exposed in create response")
	}

	var created struct {
		ID            int64  `json:"id"`
		Name          string `json:"name"`
		BaseURL       string `json:"base_url"`
		HasCredential bool   `json:"has_credential"`
	}

	if err := json.Unmarshal(
		create.Body.Bytes(),
		&created,
	); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	if created.ID <= 0 {
		t.Fatal("expected service id")
	}

	if created.Name != "Radarr" {
		t.Fatalf("unexpected name: %q", created.Name)
	}

	if created.BaseURL != "http://10.0.0.10:7878" {
		t.Fatalf(
			"unexpected URL: %q",
			created.BaseURL,
		)
	}

	if !created.HasCredential {
		t.Fatal("expected has_credential=true")
	}

	list := request(
		t,
		handler,
		sessionToken,
		http.MethodGet,
		"/api/v1/services",
		"",
	)

	if list.Code != http.StatusOK {
		t.Fatalf(
			"list returned %d: %s",
			list.Code,
			list.Body.String(),
		)
	}

	if strings.Contains(
		list.Body.String(),
		"super-secret-api-key",
	) {
		t.Fatal("credential leaked in list response")
	}

	if strings.Contains(
		list.Body.String(),
		`"credential"`,
	) {
		t.Fatal("credential field exposed in list response")
	}

	get := request(
		t,
		handler,
		sessionToken,
		http.MethodGet,
		"/api/v1/services/1",
		"",
	)

	if get.Code != http.StatusOK {
		t.Fatalf(
			"get returned %d: %s",
			get.Code,
			get.Body.String(),
		)
	}

	if strings.Contains(
		get.Body.String(),
		"super-secret-api-key",
	) {
		t.Fatal("credential leaked in get response")
	}

	updateBody := `{
		"type":"radarr",
		"name":"Movies",
		"enabled":true,
		"base_url":"http://10.0.0.11:7878",
		"update_credential":false
	}`

	update := request(
		t,
		handler,
		sessionToken,
		http.MethodPut,
		"/api/v1/services/1",
		updateBody,
	)

	if update.Code != http.StatusOK {
		t.Fatalf(
			"update returned %d: %s",
			update.Code,
			update.Body.String(),
		)
	}

	if !strings.Contains(
		update.Body.String(),
		`"has_credential":true`,
	) {
		t.Fatal("existing credential was not preserved")
	}

	remove := request(
		t,
		handler,
		sessionToken,
		http.MethodDelete,
		"/api/v1/services/1",
		"",
	)

	if remove.Code != http.StatusNoContent {
		t.Fatalf(
			"delete returned %d: %s",
			remove.Code,
			remove.Body.String(),
		)
	}

	afterDelete := request(
		t,
		handler,
		sessionToken,
		http.MethodGet,
		"/api/v1/services/1",
		"",
	)

	if afterDelete.Code != http.StatusNotFound {
		t.Fatalf(
			"expected 404 after delete, got %d",
			afterDelete.Code,
		)
	}
}

func TestServiceAPIRejectsBadInput(t *testing.T) {
	handler, sessionToken := testRouter(t)

	tests := []string{
		`{
			"type":"unknown",
			"name":"Bad",
			"enabled":true,
			"base_url":"http://localhost:1234"
		}`,
		`{
			"type":"radarr",
			"name":"",
			"enabled":true,
			"base_url":"http://localhost:7878"
		}`,
		`{
			"type":"radarr",
			"name":"Radarr",
			"enabled":true,
			"base_url":"ftp://localhost"
		}`,
		`{
			"type":"radarr",
			"name":"Radarr",
			"enabled":true,
			"base_url":"http://localhost:7878",
			"unexpected":"field"
		}`,
	}

	for _, body := range tests {
		response := request(
			t,
			handler,
			sessionToken,
			http.MethodPost,
			"/api/v1/services",
			body,
		)

		if response.Code != http.StatusBadRequest {
			t.Fatalf(
				"expected 400, got %d: %s",
				response.Code,
				response.Body.String(),
			)
		}
	}
}

func TestServiceTypesAPI(t *testing.T) {
	handler, sessionToken := testRouter(t)

	response := request(
		t,
		handler,
		sessionToken,
		http.MethodGet,
		"/api/v1/service-types",
		"",
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"service types returned %d: %s",
			response.Code,
			response.Body.String(),
		)
	}

	for _, serviceType := range []string{
		"tracearr",
		"radarr",
		"sonarr",
		"lidarr",
		"seerr",
		"tdarr",
		"qbittorrent",
		"nzbget",
	} {
		if !strings.Contains(
			response.Body.String(),
			serviceType,
		) {
			t.Fatalf(
				"missing service type %q",
				serviceType,
			)
		}
	}
}

func TestRadarrConnectionEndpoint(t *testing.T) {
	const apiKey = "saved-radarr-secret"

	radarrServer := httptest.NewServer(
		http.HandlerFunc(func(
			w http.ResponseWriter,
			r *http.Request,
		) {
			if r.URL.Path != "/api/v3/system/status" {
				t.Fatalf(
					"unexpected Radarr path: %s",
					r.URL.Path,
				)
			}

			if r.Header.Get("X-Api-Key") != apiKey {
				t.Fatal("saved API key was not sent to Radarr")
			}

			w.Header().Set(
				"Content-Type",
				"application/json",
			)

			_, _ = w.Write([]byte(`{
				"appName":"Radarr",
				"instanceName":"Movies",
				"version":"6.0.0"
			}`))
		}),
	)
	defer radarrServer.Close()

	db, err := database.Open(
		filepath.Join(t.TempDir(), "config"),
	)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if err := db.Migrate(); err != nil {
		t.Fatalf("migrate database: %v", err)
	}

	manager := services.NewManager(db)

	service, err := manager.Create(services.Input{
		Type:       models.ServiceRadarr,
		Name:       "Radarr",
		Enabled:    true,
		BaseURL:    radarrServer.URL,
		Credential: apiKey,
	})
	if err != nil {
		t.Fatalf("create service: %v", err)
	}

	registry := integrations.NewRegistry()

	if err := registry.Register(radarr.New()); err != nil {
		t.Fatalf("register Radarr: %v", err)
	}

	authManager := auth.NewManager(db)
	session, err := authManager.Setup(
		"admin",
		"correct horse battery staple",
	)
	if err != nil {
		t.Fatalf("setup administrator: %v", err)
	}

	handler := NewRouter(manager, registry, authManager)

	response := request(
		t,
		handler,
		session.Token,
		http.MethodPost,
		fmt.Sprintf(
			"/api/v1/services/%d/test",
			service.ID,
		),
		"",
	)

	if response.Code != http.StatusOK {
		t.Fatalf(
			"connection test returned %d: %s",
			response.Code,
			response.Body.String(),
		)
	}

	if strings.Contains(response.Body.String(), apiKey) {
		t.Fatal("API key leaked in connection response")
	}

	var result integrations.ConnectionResult

	if err := json.Unmarshal(
		response.Body.Bytes(),
		&result,
	); err != nil {
		t.Fatalf(
			"decode connection response: %v",
			err,
		)
	}

	if !result.OK {
		t.Fatal("expected successful connection")
	}

	if result.Version != "6.0.0" {
		t.Fatalf(
			"unexpected version: %q",
			result.Version,
		)
	}

	if result.Message != "Connected to Radarr" {
		t.Fatalf(
			"unexpected message: %q",
			result.Message,
		)
	}
}

func TestDownloadsRouteExists(t *testing.T) {
	handler, sessionToken := testRouter(t)

	request := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/downloads",
		nil,
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code == http.StatusNotFound {
		t.Fatalf(
			"GET /api/v1/downloads returned 404; route is not registered",
		)
	}

	if response.Code != http.StatusOK {
		t.Fatalf(
			"GET /api/v1/downloads returned %d, want %d",
			response.Code,
			http.StatusOK,
		)
	}
}

func TestActivityRouteExists(t *testing.T) {
	handler, sessionToken := testRouter(t)

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/activity",
		nil,
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	if response.Code == http.StatusNotFound {
		t.Fatal("GET /api/v1/activity returned 404; route is not registered")
	}

	if response.Code != http.StatusOK {
		t.Fatalf(
			"GET /api/v1/activity returned %d, want %d: %s",
			response.Code,
			http.StatusOK,
			response.Body.String(),
		)
	}

	var result struct {
		Lifecycles []models.MediaLifecycle `json:"lifecycles"`
		Errors     []services.ServiceError `json:"errors"`
	}

	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode activity response: %v", err)
	}

	if result.Lifecycles == nil {
		t.Fatal("lifecycles must be an array, not null")
	}

	if result.Errors == nil {
		t.Fatal("errors must be an array, not null")
	}
}
