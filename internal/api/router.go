package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/auth"
	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/services"
	"github.com/z354prance/overmynd/internal/webui"
)

type API struct {
	services *services.Manager
	registry *integrations.Registry
	auth     *auth.Manager
}

func NewRouter(
	serviceManager *services.Manager,
	registry *integrations.Registry,
	authManager *auth.Manager,
) http.Handler {
	api := &API{
		services: serviceManager,
		registry: registry,
		auth:     authManager,
	}

	mux := http.NewServeMux()

	// Public application and observability endpoints.
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("GET /api/v1/status", status)
	mux.HandleFunc("GET /api/v1/missing", api.missing)
	mux.HandleFunc("GET /api/v1/activity", api.activity)
	mux.HandleFunc("GET /api/v1/requests", api.requests)
	mux.HandleFunc("GET /api/v1/downloads", api.downloads)
	mux.HandleFunc("GET /api/v1/playback", api.playback)
	mux.HandleFunc("GET /api/v1/processing", api.processing)
	mux.HandleFunc("GET /api/v1/public/services", api.listPublicServices)

	// Authentication endpoints.
	mux.HandleFunc("GET /api/v1/auth/status", api.authStatus)
	mux.HandleFunc("POST /api/v1/auth/setup", api.authSetup)
	mux.HandleFunc("POST /api/v1/auth/login", api.authLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", api.authLogout)

	// Administrator-only configuration endpoints.
	mux.HandleFunc(
		"GET /api/v1/service-types",
		api.requireAdmin(api.serviceTypes),
	)

	mux.HandleFunc(
		"GET /api/v1/services",
		api.requireAdmin(api.listServices),
	)

	mux.HandleFunc(
		"POST /api/v1/services",
		api.requireAdmin(api.createService),
	)

	mux.HandleFunc(
		"GET /api/v1/services/{id}",
		api.requireAdmin(api.getService),
	)

	mux.HandleFunc(
		"PUT /api/v1/services/{id}",
		api.requireAdmin(api.updateService),
	)

	mux.HandleFunc(
		"DELETE /api/v1/services/{id}",
		api.requireAdmin(api.deleteService),
	)

	mux.HandleFunc(
		"POST /api/v1/services/{id}/test",
		api.requireAdmin(api.testService),
	)

	webFS, err := webui.Files()
	if err != nil {
		panic(err)
	}

	mux.Handle("/", http.FileServerFS(webFS))

	return protectRequests(mux)
}

func health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"application": "Overmynd",
		"version":     "dev",
		"time":        time.Now().UTC().Format(time.RFC3339),
	})
}

func status(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ok",
		"application": "Overmynd",
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		http.Error(
			w,
			"failed to encode response",
			http.StatusInternalServerError,
		)
		return
	}

	body = append(body, '\n')

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)

	_, _ = w.Write(body)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{
		"error": err.Error(),
	})
}
