// Package access is Overmynd's "need access?" flow, ported: a visitor asks for a media
// server account, an administrator approves it, the account is created as a disabled
// copy of a template user, and the visitor finishes setup from a single-use link.
// Everything the media server does is verified, and any doubt leaves the account disabled.
package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type Settings struct {
	Provider        string `json:"provider"`
	ServerPublicURL string `json:"server_public_url"`
	Enabled         bool   `json:"enabled"`
	PublicURL       string `json:"public_url"`
	EmbyURL         string `json:"emby_url"` // shared with Playback; not stored here
	EmbyKey         string `json:"emby_key,omitempty"`
	TemplateID      string `json:"template_id"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPSecurity    string `json:"smtp_security"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPassword    string `json:"smtp_password,omitempty"`
	From            string `json:"from"`
	AdminEmail      string `json:"admin_email"`
}

type Request struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Username  string `json:"username"`
	Connect   string `json:"connect_username"`
	Status    string `json:"status"`
	EmbyID    string `json:"emby_id,omitempty"`
	Error     string `json:"error,omitempty"`
	MailError string `json:"mail_error,omitempty"`
	Created   string `json:"created_at"`
	Updated   string `json:"updated_at"`
}

// rec is a request as stored, with its single-use setup link (kept only as a hash).
type rec struct {
	Request
	TokenHash    string `json:"token_hash,omitempty"`
	TokenExpires int64  `json:"token_expires,omitempty"`
}

// Manager keeps requests in one JSON file. Settings live in Overmynd's own config;
// Load and Save connect them so this package does not depend on it.
type Manager struct {
	path  string
	Load  func() Settings
	Store func(Settings) error

	mu   sync.Mutex // serialises whole operations
	fmu  sync.Mutex // guards the file and slice
	recs []rec
	next int64
	send func(Settings, string, string, letter) error
}

func New(dir string, load func() Settings, save func(Settings) error) (*Manager, error) {
	m := &Manager{path: filepath.Join(dir, "access-requests.json"), Load: load, Store: save, send: sendMail}
	b, err := os.ReadFile(m.path)
	if err == nil {
		if err = json.Unmarshal(b, &m.recs); err != nil {
			return nil, fmt.Errorf("access requests file is unreadable: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for _, r := range m.recs {
		if r.ID > m.next {
			m.next = r.ID
		}
	}
	return m, nil
}

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,63}$`)
var idPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

func address(s string) bool {
	a, e := mail.ParseAddress(s)
	return e == nil && a.Address == s && !strings.ContainsAny(s, "\r\n")
}
func validURL(s string, https bool) bool {
	u, e := url.Parse(s)
	return e == nil && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Scheme == "https" || (!https && u.Scheme == "http"))
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// persist writes the file atomically; callers hold fmu.
func (m *Manager) persist() error {
	b, err := json.MarshalIndent(m.recs, "", "  ")
	if err != nil {
		return err
	}
	tmp := m.path + ".tmp"
	if err = os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, m.path)
}

func (m *Manager) update(id int64, f func(*rec)) error {
	m.fmu.Lock()
	defer m.fmu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID == id {
			f(&m.recs[i])
			m.recs[i].Updated = now()
			return m.persist()
		}
	}
	return errors.New("request not found")
}

func (m *Manager) get(id int64) (Request, error) {
	m.fmu.Lock()
	defer m.fmu.Unlock()
	for _, r := range m.recs {
		if r.ID == id {
			return r.Request, nil
		}
	}
	return Request{}, errors.New("request not found")
}

// RecoverInterrupted runs once at startup, before serving requests. Remote writes
// cannot be rolled back; retry only the recorded user ID.
func (m *Manager) RecoverInterrupted() error {
	m.fmu.Lock()
	defer m.fmu.Unlock()
	changed := false
	for i := range m.recs {
		switch m.recs[i].Status {
		case "provisioning":
			m.recs[i].Status, m.recs[i].Error, changed = "incomplete", "Interrupted by restart; retry to finish setup", true
		case "setting_up":
			m.recs[i].Status, m.recs[i].Error, changed = "awaiting_setup", "Interrupted by restart; retry to finish setup", true
		}
	}
	if changed {
		return m.persist()
	}
	return nil
}

func (m *Manager) Settings() (Settings, error) {
	s := m.Load()
	if s.SMTPPort == 0 {
		s.SMTPPort = 465
	}
	if s.SMTPSecurity == "" {
		s.SMTPSecurity = "tls"
	}
	if s.Provider == "" {
		s.Provider = "emby"
	}
	return s, nil
}

// Validate checks settings before they are saved. Blank secrets were already filled
// from the stored values by the caller.
func (m *Manager) Validate(s Settings) (Settings, error) {
	old, _ := m.Settings()
	if s.Provider == "" {
		s.Provider = "emby"
	}
	if s.Provider != "emby" && s.Provider != "jellyfin" {
		return s, errors.New("choose Emby or Jellyfin")
	}
	if s.Enabled && s.Provider == "jellyfin" && !validURL(s.ServerPublicURL, true) {
		return s, errors.New("enter the public HTTPS Jellyfin server URL for sign-in instructions")
	}
	s.ServerPublicURL = strings.TrimRight(strings.TrimSpace(s.ServerPublicURL), "/")
	if s.ServerPublicURL != "" && !validURL(s.ServerPublicURL, true) {
		return s, errors.New("enter the media server's public address as an HTTPS URL, like https://emby.example.com")
	}
	s.PublicURL = strings.TrimRight(strings.TrimSpace(s.PublicURL), "/")
	s.EmbyURL = old.EmbyURL // the media server address and key are set on the Playback tab
	s.EmbyKey = old.EmbyKey
	if s.Enabled && (!validURL(s.PublicURL, true) || !validURL(s.EmbyURL, false) || s.EmbyKey == "" || !idPattern.MatchString(s.TemplateID) || s.SMTPHost == "" || s.SMTPPort < 1 || s.SMTPPort > 65535 || (s.SMTPSecurity != "tls" && s.SMTPSecurity != "starttls") || !address(s.From) || !address(s.AdminEmail) || s.SMTPUser == "" || s.SMTPPassword == "") {
		return s, errors.New("complete the HTTPS public URL, the media server address and key (Playback tab), the template user, and secure SMTP settings before enabling access requests")
	}
	if strings.ContainsAny(s.SMTPHost, "\r\n/: ") {
		return s, errors.New("invalid SMTP hostname")
	}
	if isJellyfin(s) != isJellyfin(old) {
		m.fmu.Lock()
		n := 0
		for _, r := range m.recs {
			if r.Status != "declined" {
				n++
			}
		}
		m.fmu.Unlock()
		if n > 0 {
			return s, errors.New("the provider cannot be changed while access requests exist")
		}
	}
	return s, nil
}

func (m *Manager) List() ([]Request, error) {
	m.fmu.Lock()
	defer m.fmu.Unlock()
	out := make([]Request, 0, len(m.recs))
	for _, r := range m.recs {
		out = append(out, r.Request)
	}
	sort.SliceStable(out, func(i, j int) bool {
		di := out[i].Status == "active" || out[i].Status == "declined"
		dj := out[j].Status == "active" || out[j].Status == "declined"
		if di != dj {
			return !di
		}
		return out[i].ID > out[j].ID
	})
	if len(out) > 500 {
		out = out[:500]
	}
	return out, nil
}

// Correct fixes contact details before provisioning completes. A username or
// recorded Emby ID is never reassigned to a different request.
func (m *Manager) Correct(id int64, email, connect string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	connect = strings.TrimSpace(connect)
	settings, err := m.Settings()
	if err != nil {
		return err
	}
	if isJellyfin(settings) {
		connect = ""
	} else {
		connect = strings.ToLower(connect)
	}
	if !address(email) || len(email) > 254 || (!isJellyfin(settings) && !address(connect)) || len(connect) > 254 {
		return errors.New("enter a valid notification email and, for Emby, the email address of your Emby Connect account")
	}
	m.fmu.Lock()
	for _, r := range m.recs {
		if r.ID != id && strings.EqualFold(r.Email, email) {
			m.fmu.Unlock()
			return errors.New("another request already uses that email")
		}
	}
	m.fmu.Unlock()
	r, e := m.get(id)
	if e != nil || (r.Status != "pending" && r.Status != "incomplete") {
		return errors.New("only pending or incomplete requests can be corrected")
	}
	return m.update(id, func(x *rec) { x.Email, x.Connect, x.Error, x.MailError = email, connect, "", "" })
}

func (m *Manager) Submit(r Request) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !s.Enabled {
		return errors.New("access requests are not enabled")
	}
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
	r.Username = strings.TrimSpace(r.Username)
	r.Connect = strings.TrimSpace(r.Connect)
	if isJellyfin(s) {
		r.Connect = ""
	} else {
		r.Connect = strings.ToLower(r.Connect)
	}
	if len(r.Name) < 1 || len(r.Name) > 100 || strings.ContainsAny(r.Name, "\r\n") || !address(r.Email) || len(r.Email) > 254 || !usernamePattern.MatchString(r.Username) || (!isJellyfin(s) && !address(r.Connect)) || len(r.Connect) > 254 {
		return errors.New("enter a name, valid email, a username of 3-64 letters/numbers/dots/dashes, and, for Emby, the email address of your Emby Connect account")
	}
	m.fmu.Lock()
	open := 0
	for _, x := range m.recs {
		if strings.EqualFold(x.Email, r.Email) || strings.EqualFold(x.Username, r.Username) {
			m.fmu.Unlock()
			return nil // Same public response, no account enumeration.
		}
		if x.Status != "active" && x.Status != "declined" {
			open++
		}
	}
	if open >= 500 {
		m.fmu.Unlock()
		return errors.New("access requests are temporarily unavailable")
	}
	m.next++
	id := m.next
	ts := now()
	m.recs = append(m.recs, rec{Request: Request{ID: id, Name: r.Name, Email: r.Email, Username: r.Username, Connect: r.Connect, Status: "pending", Created: ts, Updated: ts}})
	err := m.persist()
	m.fmu.Unlock()
	if err != nil {
		return err
	}
	m.notify(id, s, []notice{receiptNotice(r, s), adminNewNotice(r, s)})
	return nil
}

func (m *Manager) notify(id int64, s Settings, notices []notice) {
	failures := []string{}
	for _, n := range notices {
		if m.send(s, n.to, n.subject, n.l) != nil {
			failures = append(failures, "Email delivery failed for "+n.to)
		}
	}
	_ = m.update(id, func(x *rec) { x.MailError = strings.Join(failures, "; ") })
}

func (m *Manager) TestMail() error {
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !address(s.AdminEmail) || !address(s.From) {
		return errors.New("save valid sender and administrator email addresses first")
	}
	if e = m.send(s, s.AdminEmail, testNotice(s).subject, testNotice(s).l); e != nil {
		return errors.New("SMTP test failed; check host, encryption, mailbox login, and sender permissions")
	}
	return nil
}

func secret() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b[:])
}
func hash(token string) string { b := sha256.Sum256([]byte(token)); return hex.EncodeToString(b[:]) }

func (m *Manager) setState(id int64, state, message string) error {
	return m.update(id, func(x *rec) { x.Status, x.Error = state, message })
}
func (m *Manager) fail(id int64, s Settings, message string) error {
	if e := m.setState(id, "incomplete", message); e != nil {
		return e
	}
	m.notify(id, s, []notice{adminProblemNotice(id, s, message)})
	return errors.New(message)
}

// claim moves a request between states only if it is in one of the given states.
func (m *Manager) claim(id int64, to string, from ...string) bool {
	m.fmu.Lock()
	defer m.fmu.Unlock()
	for i := range m.recs {
		if m.recs[i].ID != id {
			continue
		}
		for _, f := range from {
			if m.recs[i].Status == f {
				m.recs[i].Status, m.recs[i].Error, m.recs[i].Updated = to, "", now()
				return m.persist() == nil
			}
		}
	}
	return false
}

func (m *Manager) Approve(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !s.Enabled {
		return errors.New("access requests are disabled")
	}
	r, e := m.get(id)
	if e != nil {
		return errors.New("request not found")
	}
	if r.Status != "pending" && r.Status != "incomplete" {
		return errors.New("request is not awaiting approval or retry")
	}
	if !m.claim(id, "provisioning", "pending", "incomplete") {
		return errors.New("request already being handled")
	}
	template, e := readTemplate(ctx, s)
	if e != nil {
		return m.fail(id, s, e.Error())
	}
	// Never adopt an existing username after an uncertain create result.
	createdNow := false
	if r.EmbyID == "" {
		var users []embyUser
		if e = emby(ctx, s, "GET", "/Users", nil, &users); e != nil {
			return m.fail(id, s, "Unable to check existing media server usernames")
		}
		for _, u := range users {
			if strings.EqualFold(u.Name, r.Username) {
				return m.fail(id, s, "Username already exists on the media server; no existing account was changed. Resolve the conflict on the media server before retrying")
			}
		}
		var created embyUser
		createBody := map[string]any{"Name": r.Username, "CopyFromUserId": s.TemplateID, "UserCopyOptions": []string{"UserPolicy", "UserConfiguration"}}
		if isJellyfin(s) {
			createBody = map[string]any{"Name": r.Username, "Password": secret()}
		}
		e = emby(ctx, s, "POST", "/Users/New", createBody, &created)
		if e != nil || !idPattern.MatchString(created.ID) {
			return m.fail(id, s, "Account creation did not return a confirmed user ID. Check the media server for a partially created account before retrying")
		}
		r.EmbyID = created.ID
		createdNow = true
		if e = m.update(id, func(x *rec) { x.EmbyID = created.ID }); e != nil {
			return e
		}
	}
	var user embyUser
	if e = emby(ctx, s, "GET", "/Users/"+url.PathEscape(r.EmbyID), nil, &user); e != nil {
		return m.fail(id, s, "Unable to verify the new media server user")
	}
	if user.ID != r.EmbyID || !strings.EqualFold(user.Name, r.Username) {
		return m.fail(id, s, "Media server user identity changed; inspect it before retrying")
	}
	if user.Policy == nil {
		return m.fail(id, s, "Media server did not return the user policy")
	}
	if createdNow && user.Policy["IsDisabled"] != true {
		if err := quarantine(s, r, user.Policy); err != nil {
			return m.fail(id, s, err.Error())
		}
		// The disable was read back successfully. Continue on this same user;
		// never repeat creation or skip explicit template verification.
	}
	if _, _, e = applyTemplate(ctx, s, r, template); e != nil {
		return m.fail(id, s, e.Error())
	}
	if e = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Password", map[string]any{"Id": r.EmbyID, "NewPw": secret()}, nil); e != nil {
		return m.fail(id, s, "Unable to secure the new account with a temporary random password")
	}
	if !isJellyfin(s) {
		var link struct{ IsPending *bool }
		if e = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Connect/Link?ConnectUsername="+url.QueryEscape(r.Connect), nil, &link); e != nil || link.IsPending == nil {
			return m.fail(id, s, "Emby Connect linking failed; check that the Connect email address is right")
		}
	}
	if e = m.setState(id, "awaiting_setup", ""); e != nil {
		return e
	}
	return m.sendSetup(r, s)
}

func (m *Manager) sendSetup(r Request, s Settings) error {
	token := secret()
	if e := m.update(r.ID, func(x *rec) { x.TokenHash, x.TokenExpires = hash(token), time.Now().Add(24*time.Hour).Unix() }); e != nil {
		return e
	}
	m.notify(r.ID, s, []notice{setupNotice(r, s, token)})
	return nil
}

func (m *Manager) Resend(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.Settings()
	if e != nil {
		return e
	}
	r, e := m.get(id)
	if e != nil {
		return e
	}
	switch r.Status {
	case "awaiting_setup":
		return m.sendSetup(r, s)
	case "pending":
		m.notify(id, s, []notice{receiptNotice(r, s), adminPendingNotice(r, s)})
	case "declined":
		m.notify(id, s, []notice{declinedNotice(r)})
	case "active":
		m.notify(id, s, []notice{serverCompletionNotice(r, s)})
	default:
		return errors.New("resolve the setup error before resending")
	}
	return nil
}

func (m *Manager) Decline(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !m.claim(id, "declined", "pending") {
		return errors.New("only pending requests can be declined")
	}
	r, e := m.get(id)
	if e != nil {
		return e
	}
	m.notify(id, s, []notice{declinedNotice(r)})
	return nil
}

func (m *Manager) Setup(ctx context.Context, token, password string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if len(token) != 64 || len(password) < 12 || len(password) > 256 {
		return errors.New("a valid setup link and a password of 12-256 characters are required")
	}
	var id int64
	m.fmu.Lock()
	for _, x := range m.recs {
		if x.TokenHash == hash(token) && x.TokenExpires > time.Now().Unix() && x.Status == "awaiting_setup" {
			id = x.ID
		}
	}
	m.fmu.Unlock()
	if id == 0 {
		return errors.New("setup link is invalid, expired, or already used")
	}
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !s.Enabled {
		return errors.New("account setup is temporarily disabled")
	}
	r, e := m.get(id)
	if e != nil {
		return e
	}
	if !m.claim(id, "setting_up", "awaiting_setup") {
		return errors.New("setup is already in progress")
	}
	success := false
	activationAttempted := false
	defer func() {
		if !success {
			recoveryMessage := "Setup did not finish; the recipient can retry their link"
			if activationAttempted {
				if err := quarantine(s, r, nil); err != nil {
					recoveryMessage = err.Error()
				}
			}
			_ = m.setState(id, "awaiting_setup", recoveryMessage)
			if r.Error == "" || activationAttempted {
				m.notify(id, s, []notice{adminStuckNotice(id, s, recoveryMessage)})
			}
		}
	}()
	var user embyUser
	if e = emby(ctx, s, "GET", "/Users/"+r.EmbyID, nil, &user); e != nil || user.Policy == nil {
		return errors.New("unable to read your media server account")
	}
	if user.ID != r.EmbyID || !strings.EqualFold(user.Name, r.Username) {
		return errors.New("account identity changed; contact the server owner")
	}
	if user.Policy["IsAdministrator"] != false {
		return errors.New("account permissions changed; contact the server owner")
	}
	if user.Policy["IsDisabled"] != true {
		if e = quarantine(s, r, user.Policy); e != nil {
			return e
		}
	}
	template, e := readTemplate(ctx, s)
	if e != nil {
		return e
	}
	desiredPolicy, desiredConfig, e := applyTemplate(ctx, s, r, template)
	if e != nil {
		return e
	}
	if e = verifyConnect(ctx, s, r, user); e != nil {
		return e
	}
	if e = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Password", map[string]any{"Id": r.EmbyID, "NewPw": password}, nil); e != nil {
		return errors.New("The media server could not set the password; retry or contact the server owner")
	}
	desiredPolicy["IsDisabled"] = false
	activationAttempted = true
	if e = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Policy", desiredPolicy, nil); e != nil {
		return errors.New("unable to enable your account; contact the server owner")
	}
	if e = verifySettings(ctx, s, r, desiredPolicy, desiredConfig); e != nil {
		return e
	}
	if e = m.update(id, func(x *rec) { x.Status, x.Error, x.TokenHash, x.TokenExpires = "active", "", "", 0 }); e != nil {
		return e
	}
	success = true
	m.notify(id, s, []notice{serverCompletionNotice(r, s)})
	return nil
}
