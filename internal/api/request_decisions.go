package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

// Decisions use the saved Seerr API key only after Overmynd admin authentication.
func (a *API) decideRequest(w http.ResponseWriter, r *http.Request) {
	serviceID, e1 := strconv.ParseInt(r.PathValue("serviceID"), 10, 64)
	requestID, e2 := strconv.ParseInt(r.PathValue("requestID"), 10, 64)
	action := r.PathValue("action")
	if e1 != nil || e2 != nil || serviceID <= 0 || requestID <= 0 || (action != "approve" && action != "decline") {
		writeError(w, 400, errors.New("invalid request decision"))
		return
	}
	service, err := a.services.Get(serviceID)
	if err != nil || !service.Enabled || service.Type != models.ServiceSeerr {
		writeError(w, 404, errors.New("Seerr service unavailable"))
		return
	}
	key, err := a.services.Credential(serviceID)
	if err != nil || key == "" {
		writeError(w, 503, errors.New("Seerr credentials unavailable"))
		return
	}
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	endpoint := fmt.Sprintf("%s/api/v1/request/%d", service.BaseURL, requestID)
	call := func(method, url string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(r.Context(), method, url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-Api-Key", key)
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Content-Type", "application/json")
		return client.Do(req)
	}
	resp, err := call("GET", endpoint)
	if err != nil {
		writeError(w, 502, errors.New("unable to check request in Seerr"))
		return
	}
	var current struct {
		Status int `json:"status"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&current)
	resp.Body.Close()
	if resp.StatusCode != 200 || err != nil {
		writeError(w, 502, errors.New("unable to check request in Seerr"))
		return
	}
	if current.Status != 1 {
		writeError(w, 409, errors.New("request is no longer awaiting approval; refresh the list"))
		return
	}
	resp, err = call("POST", endpoint+"/"+action)
	if err != nil {
		writeError(w, 502, errors.New("could not confirm decision; check Seerr before retrying"))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode == 409 {
		writeError(w, 409, errors.New("request is no longer awaiting approval; refresh the list"))
		return
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, 502, errors.New("Seerr could not complete the decision; check its permissions and request status"))
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
