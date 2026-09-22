package api

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/integrations/seerr"
	"github.com/z354prance/overmynd/internal/models"
	"github.com/z354prance/overmynd/internal/services"
)

type requestLimit struct {
	count int
	until time.Time
}
type publicRequestLimiter struct {
	mu      sync.Mutex
	clients map[string]requestLimit
}

func (l *publicRequestLimiter) allow(r *http.Request) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.clients == nil {
		l.clients = make(map[string]requestLimit)
	}
	now := time.Now()
	for key, item := range l.clients {
		if now.After(item.until) {
			delete(l.clients, key)
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	item := l.clients[ip]
	if item.count >= 10 || (item.count == 0 && len(l.clients) >= 1024) {
		return false
	}
	if item.count == 0 {
		item.until = now.Add(time.Minute)
	}
	item.count++
	l.clients[ip] = item
	return true
}

func (a *API) requestService(settings services.RequestSettings) (models.Service, string, error) {
	service, err := a.services.Get(settings.ServiceID)
	if err != nil || !service.Enabled || service.Type != models.ServiceSeerr || settings.UserID <= 0 {
		return models.Service{}, "", errors.New("media requests are not configured")
	}
	key, err := a.services.Credential(service.ID)
	if err != nil || strings.TrimSpace(key) == "" {
		return models.Service{}, "", errors.New("media requests are not configured")
	}
	return service, key, nil
}

func (a *API) getRequestSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := a.services.RequestSettings()
	if err != nil {
		writeError(w, 500, errors.New("unable to read request settings"))
		return
	}
	writeJSON(w, 200, settings)
}

func (a *API) saveRequestSettings(w http.ResponseWriter, r *http.Request) {
	var settings services.RequestSettings
	if err := decodeJSON(r, &settings); err != nil {
		writeError(w, 400, err)
		return
	}
	if settings.Enabled {
		service, key, err := a.requestService(settings)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		var user struct {
			ID int64 `json:"id"`
		}
		err = seerr.UserJSON(r.Context(), service.BaseURL, key, settings.UserID, "GET", "auth/me", nil, &user)
		if err != nil {
			writeError(w, 400, err)
			return
		}
		if user.ID != settings.UserID {
			writeError(w, 400, errors.New("Seerr did not authenticate as the selected user"))
			return
		}
	}
	if err := a.services.SaveRequestSettings(settings); err != nil {
		writeError(w, 500, errors.New("unable to save request settings"))
		return
	}
	writeJSON(w, 200, settings)
}

func (a *API) mediaRequestStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := a.services.RequestSettings()
	if err != nil {
		writeError(w, 503, errors.New("media requests are unavailable"))
		return
	}
	_, _, err = a.requestService(settings)
	writeJSON(w, 200, map[string]bool{"enabled": settings.Enabled && err == nil})
}

func (a *API) publicSeerr(w http.ResponseWriter, r *http.Request) (services.RequestSettings, models.Service, string, bool) {
	settings, err := a.services.RequestSettings()
	if err != nil || !settings.Enabled {
		writeError(w, 503, errors.New("media requests are not enabled"))
		return settings, models.Service{}, "", false
	}
	service, key, err := a.requestService(settings)
	if err != nil {
		writeError(w, 503, err)
		return settings, service, key, false
	}
	return settings, service, key, true
}

func (a *API) searchMedia(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if len(query) < 2 || len(query) > 200 {
		writeError(w, 400, errors.New("enter between 2 and 200 characters"))
		return
	}
	settings, service, key, ok := a.publicSeerr(w, r)
	if !ok {
		return
	}
	var result struct {
		Results []seerr.SearchResult `json:"results"`
	}
	err := seerr.UserJSON(r.Context(), service.BaseURL, key, settings.UserID, "GET", "search?"+strings.ReplaceAll(url.Values{"query": {query}, "page": {"1"}}.Encode(), "+", "%20"), nil, &result)
	if err != nil {
		writeError(w, 502, err)
		return
	}
	writeJSON(w, 200, map[string]any{"results": seerr.SanitizeResults(result.Results)})
}

func (a *API) createMediaRequest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		MediaID    int64  `json:"media_id"`
		MediaType  string `json:"media_type"`
		AllSeasons bool   `json:"all_seasons"`
	}
	if err := decodeJSON(r, &input); err != nil {
		writeError(w, 400, err)
		return
	}
	if input.MediaID <= 0 || (input.MediaType != "movie" && input.MediaType != "tv") || (input.MediaType == "tv" && !input.AllSeasons) {
		writeError(w, 400, errors.New("select a movie or confirm all seasons of a TV show"))
		return
	}
	settings, service, key, ok := a.publicSeerr(w, r)
	if !ok {
		return
	}
	if !a.requestLimiter.allow(r) {
		w.Header().Set("Retry-After", "60")
		writeError(w, 429, errors.New("too many requests; try again in a minute"))
		return
	}
	// No caller-controlled user, approval, quota, server or profile overrides.
	payload := map[string]any{"mediaId": input.MediaID, "mediaType": input.MediaType}
	if input.MediaType == "tv" {
		payload["seasons"] = "all"
	}
	var response struct {
		ID     int64 `json:"id"`
		Status int   `json:"status"`
	}
	err := seerr.UserJSON(r.Context(), service.BaseURL, key, settings.UserID, "POST", "request", payload, &response)
	if err != nil {
		writeError(w, 502, err)
		return
	}
	if response.ID <= 0 {
		writeError(w, 502, errors.New("Seerr did not confirm a request; check Seerr before retrying"))
		return
	}
	writeJSON(w, 201, response)
}
