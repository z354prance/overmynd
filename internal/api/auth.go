package api

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/auth"
)

const sessionCookieName = "overmynd_session"

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (a *API) authStatus(w http.ResponseWriter, r *http.Request) {
	setupRequired, err := a.auth.SetupRequired()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response := map[string]any{
		"setup_required": setupRequired,
		"authenticated":  false,
	}

	token := sessionToken(r)
	if token != "" {
		user, err := a.auth.Authenticate(token)
		if err == nil {
			response["authenticated"] = true
			response["username"] = user.Username
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *API) authSetup(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("cross-origin request denied"))
		return
	}

	var request authRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	session, err := a.auth.Setup(request.Username, request.Password)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, auth.ErrSetupComplete) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}

	setSessionCookie(w, r, session.Token, session.ExpiresAt)
	writeJSON(w, http.StatusCreated, map[string]any{
		"authenticated": true,
		"username":      session.User.Username,
	})
}

func (a *API) authLogin(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("cross-origin request denied"))
		return
	}

	var request authRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	session, err := a.auth.Login(request.Username, request.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			writeError(w, http.StatusUnauthorized, auth.ErrInvalidCredentials)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	setSessionCookie(w, r, session.Token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": true,
		"username":      session.User.Username,
	})
}

func (a *API) authLogout(w http.ResponseWriter, r *http.Request) {
	if !sameOriginRequest(r) {
		writeError(w, http.StatusForbidden, errors.New("cross-origin request denied"))
		return
	}

	token := sessionToken(r)
	if err := a.auth.Logout(token); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	clearSessionCookie(w, r)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		if _, err := a.auth.Authenticate(token); err != nil {
			writeError(w, http.StatusUnauthorized, auth.ErrNotAuthenticated)
			return
		}

		if isStateChanging(r.Method) && !sameOriginRequest(r) {
			writeError(w, http.StatusForbidden, errors.New("cross-origin request denied"))
			return
		}

		next(w, r)
	}
}

func isStateChanging(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// sameOriginRequest rejects browser cross-site writes while allowing requests
// without Origin/Referer (curl, local health tooling, and same-origin clients
// that omit them). When either header is present it must match the externally
// visible request scheme and host.
func sameOriginRequest(r *http.Request) bool {
	source := strings.TrimSpace(r.Header.Get("Origin"))
	if source == "" {
		source = strings.TrimSpace(r.Header.Get("Referer"))
	}
	if source == "" {
		return true
	}

	u, err := url.Parse(source)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return false
	}

	expectedScheme := "http"
	if requestIsHTTPS(r) {
		expectedScheme = "https"
	}

	return strings.EqualFold(u.Scheme, expectedScheme) &&
		strings.EqualFold(u.Host, r.Host)
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expiresAt time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   requestIsHTTPS(r),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(1, 0),
		MaxAge:   -1,
	})
}

func requestIsHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https")
}
