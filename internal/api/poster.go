package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

func (a *API) posterSignature(id, path string) string {
	h := hmac.New(sha256.New, a.posterKey[:])
	h.Write([]byte(id + "\n" + path))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func validPosterPath(path string) bool {
	u, err := url.Parse(path)
	return err == nil && !u.IsAbs() && u.Host == "" && u.User == nil && u.Fragment == "" &&
		u.Path == "/api/v1/images/proxy" && u.RawPath == "" && len(path) < 4096
}

func (a *API) posterURL(serviceID int64, path string) string {
	if path == "" {
		return ""
	}
	// Tracearr sends a relative proxy URL; it cannot be loaded against Overmynd
	// directly. Only sign this known image endpoint, never arbitrary source URLs.
	if !validPosterPath(path) {
		return ""
	}
	id := strconv.FormatInt(serviceID, 10)
	return "/api/v1/playback/poster?" + url.Values{"service_id": {id}, "path": {path}, "signature": {a.posterSignature(id, path)}}.Encode()
}

func (a *API) playbackPoster(w http.ResponseWriter, r *http.Request) {
	id, path, signature := r.URL.Query().Get("service_id"), r.URL.Query().Get("path"), r.URL.Query().Get("signature")
	if !validPosterPath(path) || !hmac.Equal([]byte(signature), []byte(a.posterSignature(id, path))) {
		http.NotFound(w, r)
		return
	}
	serviceID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	service, err := a.services.Get(serviceID)
	if err != nil || !service.Enabled || service.Type != models.ServiceTracearr {
		http.NotFound(w, r)
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "GET", strings.TrimRight(service.BaseURL, "/")+path, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	// Tracearr's image proxy is public. Never send its API token to image URLs.
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		http.NotFound(w, r)
		return
	}
	const maxImage = 5 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil || len(body) > maxImage {
		http.NotFound(w, r)
		return
	}
	contentType := http.DetectContentType(body)
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(body)
}
