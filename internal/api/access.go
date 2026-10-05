package api

import (
	"errors"
	"github.com/z354prance/overmynd/internal/access"
	"net/http"
	"strconv"
	"time"
)

func (a *API) accessStatus(w http.ResponseWriter, r *http.Request) {
	s, e := a.access.Settings()
	writeJSON(w, 200, map[string]bool{"enabled": e == nil && s.Enabled})
}
func (a *API) accessSettings(w http.ResponseWriter, r *http.Request) {
	s, e := a.access.Settings()
	if e != nil {
		writeError(w, 500, errors.New("unable to load access settings"))
		return
	}
	writeJSON(w, 200, access.Redact(s))
}
func (a *API) accessSaveSettings(w http.ResponseWriter, r *http.Request) {
	var s access.Settings
	if e := decodeJSON(r, &s); e != nil {
		writeError(w, 400, e)
		return
	}
	if e := a.access.Save(s); e != nil {
		writeError(w, 400, e)
		return
	}
	a.accessSettings(w, r)
}
func (a *API) accessList(w http.ResponseWriter, r *http.Request) {
	items, e := a.access.List()
	if e != nil {
		writeError(w, 500, errors.New("unable to load requests"))
		return
	}
	writeJSON(w, 200, map[string]any{"requests": items})
}
func (a *API) accessTestEmail(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if e := a.access.TestMail(); e != nil {
		writeError(w, 400, e)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Test email sent"})
}
func (a *API) accessSubmit(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if !a.accessLimiter.allow(r) {
		writeError(w, 429, errors.New("too many requests; please try again later"))
		return
	}
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Connect  string `json:"connect_username"`
	}
	if e := decodeJSON(r, &input); e != nil {
		writeError(w, 400, e)
		return
	}
	if e := a.access.Submit(access.Request{Name: input.Name, Email: input.Email, Username: input.Username, Connect: input.Connect}); e != nil {
		writeError(w, 400, e)
		return
	}
	writeJSON(w, 202, map[string]string{"message": "If this is a new request, it has been submitted for approval. Watch your email for updates."})
}
func (a *API) accessSetup(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if !a.accessLimiter.allow(r) {
		writeError(w, 429, errors.New("too many attempts; try again later"))
		return
	}
	var input struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if e := decodeJSON(r, &input); e != nil {
		writeError(w, 400, e)
		return
	}
	if e := a.access.Setup(r.Context(), input.Token, input.Password); e != nil {
		writeError(w, 400, e)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Your account is ready. You can now sign in to Emby."})
}
func (a *API) accessAction(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		writeError(w, 400, errors.New("invalid request"))
		return
	}
	switch r.PathValue("action") {
	case "correct":
		var input struct {
			Email   string `json:"email"`
			Connect string `json:"connect_username"`
		}
		if e = decodeJSON(r, &input); e == nil {
			e = a.access.Correct(id, input.Email, input.Connect)
		}
	case "approve":
		e = a.access.Approve(r.Context(), id)
	case "decline":
		e = a.access.Decline(id)
	case "resend":
		e = a.access.Resend(id)
	default:
		writeError(w, 404, errors.New("unknown action"))
		return
	}
	if e != nil {
		writeError(w, 400, e)
		return
	}
	writeJSON(w, 200, map[string]string{"message": "Request updated; check its status and email delivery result"})
}
