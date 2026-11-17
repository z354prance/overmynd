package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/z354prance/overmynd/internal/access"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/secret"
)

var accessLimit requestLimiter

// NewAccess connects the access manager to the saved settings. The media server
// address and key are the ones on the Playback tab.
func NewAccess(dir string, cfg *config.Config) (*access.Manager, error) {
	load := func() access.Settings {
		c := cfg.Get()
		a := c.Access
		return access.Settings{Provider: a.Provider, ServerPublicURL: a.ServerPublicURL, Enabled: a.Enabled, PublicURL: a.PublicURL,
			EmbyURL: c.Playback.EmbyURL, EmbyKey: string(c.Playback.EmbyKey), TemplateID: a.TemplateID, SMTPHost: a.SMTPHost, SMTPPort: a.SMTPPort,
			SMTPSecurity: a.SMTPSecurity, SMTPUser: a.SMTPUser, SMTPPassword: string(a.SMTPPassword), From: a.From, AdminEmail: a.AdminEmail}
	}
	store := func(v access.Settings) error {
		c := cfg.Get()
		c.Access = config.Access{Provider: v.Provider, ServerPublicURL: v.ServerPublicURL, Enabled: v.Enabled, PublicURL: v.PublicURL, TemplateID: v.TemplateID,
			SMTPHost: v.SMTPHost, SMTPPort: v.SMTPPort, SMTPSecurity: v.SMTPSecurity, SMTPUser: v.SMTPUser, SMTPPassword: secret.String(v.SMTPPassword), From: v.From, AdminEmail: v.AdminEmail}
		return cfg.Set(c)
	}
	return access.New(dir, load, store)
}

type accessView struct {
	Provider        string `json:"provider"`
	ServerPublicURL string `json:"server_public_url"`
	Enabled         bool   `json:"enabled"`
	PublicURL       string `json:"public_url"`
	TemplateID      string `json:"template_id"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPSecurity    string `json:"smtp_security"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPassword    string `json:"smtp_password,omitempty"` // write-only
	HasPassword     bool   `json:"has_smtp_password"`
	From            string `json:"from"`
	AdminEmail      string `json:"admin_email"`
}

func (s *Server) accessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/access/status", s.accessStatus)
	mux.HandleFunc("POST /api/v1/access/request", s.guard(s.accessSubmit))
	mux.HandleFunc("POST /api/v1/access/setup", s.guard(s.accessSetup))
	mux.HandleFunc("GET /admin/api/access", s.admin(s.accessSettings))
	mux.HandleFunc("PUT /admin/api/access", s.guard(s.admin(s.accessSave)))
	mux.HandleFunc("POST /admin/api/access/test-email", s.guard(s.admin(s.accessTestEmail)))
	mux.HandleFunc("GET /admin/api/access/requests", s.admin(s.accessList))
	mux.HandleFunc("POST /admin/api/access/requests/{id}/{action}", s.guard(s.admin(s.accessAction)))
}

func (s *Server) accessStatus(w http.ResponseWriter, r *http.Request) {
	if s.Access == nil {
		writeJSON(w, map[string]any{"enabled": false})
		return
	}
	v, _ := s.Access.Settings()
	writeJSON(w, map[string]any{"enabled": v.Enabled, "provider": v.Provider})
}

func (s *Server) accessReady(w http.ResponseWriter) bool {
	if s.Access == nil {
		jsonErr(w, http.StatusServiceUnavailable, "access requests are unavailable")
		return false
	}
	return true
}

func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if json.NewDecoder(r.Body).Decode(v) != nil {
		jsonErr(w, http.StatusBadRequest, "invalid request")
		return false
	}
	return true
}

func (s *Server) accessSubmit(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if !accessLimit.allow(r) {
		jsonErr(w, http.StatusTooManyRequests, "too many requests; please try again later")
		return
	}
	var in struct{ Name, Email, Username, Connect string }
	var raw struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Username string `json:"username"`
		Connect  string `json:"connect_username"`
	}
	if !readBody(w, r, &raw) {
		return
	}
	in.Name, in.Email, in.Username, in.Connect = raw.Name, raw.Email, raw.Username, raw.Connect
	if err := s.Access.Submit(access.Request{Name: in.Name, Email: in.Email, Username: in.Username, Connect: in.Connect}); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusAccepted)
	writeJSON(w, map[string]string{"message": "If this is a new request, it has been submitted for approval. Watch your email for updates."})
}

func (s *Server) accessSetup(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if !accessLimit.allow(r) {
		jsonErr(w, http.StatusTooManyRequests, "too many attempts; try again later")
		return
	}
	var in struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !readBody(w, r, &in) {
		return
	}
	if err := s.Access.Setup(r.Context(), in.Token, in.Password); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "Your account is ready. You can now sign in to your media server."})
}

func (s *Server) accessView() (accessView, error) {
	v, err := s.Access.Settings()
	return accessView{v.Provider, v.ServerPublicURL, v.Enabled, v.PublicURL, v.TemplateID, v.SMTPHost, v.SMTPPort, v.SMTPSecurity, v.SMTPUser, "", v.SMTPPassword != "", v.From, v.AdminEmail}, err
}

func (s *Server) accessSettings(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	v, err := s.accessView()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "unable to load access settings")
		return
	}
	writeJSON(w, v)
}

func (s *Server) accessSave(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	var in accessView
	if !readBody(w, r, &in) {
		return
	}
	old, _ := s.Access.Settings()
	next := old
	next.Provider, next.ServerPublicURL, next.Enabled, next.PublicURL, next.TemplateID = in.Provider, in.ServerPublicURL, in.Enabled, in.PublicURL, in.TemplateID
	next.SMTPHost, next.SMTPPort, next.SMTPSecurity, next.SMTPUser, next.From, next.AdminEmail = in.SMTPHost, in.SMTPPort, in.SMTPSecurity, in.SMTPUser, in.From, in.AdminEmail
	if in.SMTPPassword != "" {
		next.SMTPPassword = in.SMTPPassword
	}
	next, err := s.Access.Validate(next)
	if err == nil {
		err = s.Access.Store(next)
	}
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	s.accessSettings(w, r)
}

func (s *Server) accessTestEmail(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	if err := s.Access.TestMail(); err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "Test email sent"})
}

func (s *Server) accessList(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	items, err := s.Access.List()
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "unable to load requests")
		return
	}
	writeJSON(w, map[string]any{"requests": items})
}

func (s *Server) accessAction(w http.ResponseWriter, r *http.Request) {
	if !s.accessReady(w) {
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(120 * time.Second))
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		jsonErr(w, http.StatusBadRequest, "invalid request")
		return
	}
	var ctx context.Context = r.Context()
	switch r.PathValue("action") {
	case "correct":
		var in struct {
			Email   string `json:"email"`
			Connect string `json:"connect_username"`
		}
		if !readBody(w, r, &in) {
			return
		}
		err = s.Access.Correct(id, in.Email, in.Connect)
	case "reapply-template":
		err = s.Access.ReapplyTemplate(ctx, id)
	case "reset":
		err = s.Access.ResetDeleted(ctx, id)
	case "approve":
		err = s.Access.Approve(ctx, id)
	case "decline":
		err = s.Access.Decline(id)
	case "resend":
		err = s.Access.Resend(id)
	default:
		err = errors.New("unknown action")
	}
	if err != nil {
		jsonErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "Request updated; check its status and email delivery result"})
}
