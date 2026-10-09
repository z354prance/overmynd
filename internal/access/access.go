package access

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Settings struct {
	Enabled         bool   `json:"enabled"`
	PublicURL       string `json:"public_url"`
	EmbyURL         string `json:"emby_url"`
	EmbyKey         string `json:"emby_key,omitempty"`
	TemplateID      string `json:"template_id"`
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPSecurity    string `json:"smtp_security"`
	SMTPUser        string `json:"smtp_user"`
	SMTPPassword    string `json:"smtp_password,omitempty"`
	From            string `json:"from"`
	AdminEmail      string `json:"admin_email"`
	HasEmbyKey      bool   `json:"has_emby_key"`
	HasSMTPPassword bool   `json:"has_smtp_password"`
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
type Manager struct {
	db   *sql.DB
	mu   sync.Mutex
	send func(Settings, string, string, string) error
}

func New(db *sql.DB) *Manager { return &Manager{db: db, send: sendMail} }

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

// RecoverInterrupted runs once at startup, before serving requests. Remote writes
// cannot share a transaction with SQLite; retry only the recorded user ID.
func RecoverInterrupted(db *sql.DB) error {
	_, err := db.Exec("UPDATE access_requests SET status=CASE status WHEN 'provisioning' THEN 'incomplete' ELSE 'awaiting_setup' END,error='Interrupted by restart; retry to finish setup' WHERE status IN ('provisioning','setting_up')")
	return err
}
func (m *Manager) Settings() (Settings, error) {
	var s Settings
	var value string
	e := m.db.QueryRow("SELECT value FROM settings WHERE key='access_settings'").Scan(&value)
	if errors.Is(e, sql.ErrNoRows) {
		s.SMTPPort = 465
		s.SMTPSecurity = "tls"
		return s, nil
	}
	if e != nil {
		return s, e
	}
	e = json.Unmarshal([]byte(value), &s)
	return s, e
}
func Redact(s Settings) Settings {
	s.HasEmbyKey = s.EmbyKey != ""
	s.HasSMTPPassword = s.SMTPPassword != ""
	s.EmbyKey = ""
	s.SMTPPassword = ""
	return s
}
func (m *Manager) Save(s Settings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	old, e := m.Settings()
	if e != nil {
		return e
	}
	if s.EmbyKey == "" {
		s.EmbyKey = old.EmbyKey
	}
	if s.SMTPPassword == "" {
		s.SMTPPassword = old.SMTPPassword
	}
	s.EmbyURL = strings.TrimRight(strings.TrimSpace(s.EmbyURL), "/")
	s.PublicURL = strings.TrimRight(strings.TrimSpace(s.PublicURL), "/")
	if s.Enabled && (!validURL(s.PublicURL, true) || !validURL(s.EmbyURL, false) || s.EmbyKey == "" || !idPattern.MatchString(s.TemplateID) || s.SMTPHost == "" || s.SMTPPort < 1 || s.SMTPPort > 65535 || (s.SMTPSecurity != "tls" && s.SMTPSecurity != "starttls") || !address(s.From) || !address(s.AdminEmail) || s.SMTPUser == "" || s.SMTPPassword == "") {
		return errors.New("complete the HTTPS public URL, Emby template, and secure SMTP settings before enabling access requests")
	}
	if strings.ContainsAny(s.SMTPHost, "\r\n/: ") {
		return errors.New("invalid SMTP hostname")
	}
	if old.EmbyURL != "" && s.EmbyURL != old.EmbyURL {
		var n int
		if e = m.db.QueryRow("SELECT COUNT(*) FROM access_requests WHERE emby_id<>'' OR status='provisioning'").Scan(&n); e != nil {
			return e
		}
		if n > 0 {
			return errors.New("Emby server cannot be changed while provisioned requests exist")
		}
	}
	b, e := json.Marshal(s)
	if e != nil {
		return e
	}
	_, e = m.db.Exec("INSERT INTO settings(key,value,updated_at) VALUES('access_settings',?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at", string(b), time.Now().UTC().Format(time.RFC3339))
	return e
}
func (m *Manager) List() ([]Request, error) {
	rows, e := m.db.Query("SELECT id,name,email,username,connect_username,status,emby_id,error,mail_error,created_at,updated_at FROM access_requests ORDER BY CASE WHEN status IN ('active','declined') THEN 1 ELSE 0 END,id DESC LIMIT 500")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Request{}
	for rows.Next() {
		var r Request
		if e = rows.Scan(&r.ID, &r.Name, &r.Email, &r.Username, &r.Connect, &r.Status, &r.EmbyID, &r.Error, &r.MailError, &r.Created, &r.Updated); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func (m *Manager) get(id int64) (Request, error) {
	var r Request
	e := m.db.QueryRow("SELECT id,name,email,username,connect_username,status,emby_id,error,mail_error,created_at,updated_at FROM access_requests WHERE id=?", id).Scan(&r.ID, &r.Name, &r.Email, &r.Username, &r.Connect, &r.Status, &r.EmbyID, &r.Error, &r.MailError, &r.Created, &r.Updated)
	return r, e
}

// Correct fixes contact details before provisioning completes. A username or
// recorded Emby ID is never reassigned to a different request.
func (m *Manager) Correct(id int64, email, connect string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	email = strings.ToLower(strings.TrimSpace(email))
	connect = strings.TrimSpace(connect)
	if !address(email) || len(email) > 254 || len(connect) < 1 || len(connect) > 100 || strings.ContainsAny(connect, "\r\n") {
		return errors.New("enter a valid email and Emby Connect username")
	}
	var duplicates int
	if e := m.db.QueryRow("SELECT COUNT(*) FROM access_requests WHERE email=? AND id<>?", email, id).Scan(&duplicates); e != nil {
		return e
	}
	if duplicates > 0 {
		return errors.New("another request already uses that email")
	}
	result, e := m.db.Exec("UPDATE access_requests SET email=?,connect_username=?,error='',mail_error='',updated_at=? WHERE id=? AND status IN ('pending','incomplete')", email, connect, time.Now().UTC().Format(time.RFC3339), id)
	if e != nil {
		return e
	}
	n, e := result.RowsAffected()
	if e != nil {
		return e
	}
	if n != 1 {
		return errors.New("only pending or incomplete requests can be corrected")
	}
	return nil
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
	if len(r.Name) < 1 || len(r.Name) > 100 || strings.ContainsAny(r.Name, "\r\n") || !address(r.Email) || len(r.Email) > 254 || !usernamePattern.MatchString(r.Username) || len(r.Connect) < 1 || len(r.Connect) > 100 || strings.ContainsAny(r.Connect, "\r\n") {
		return errors.New("enter a name, valid email, a username of 3-64 letters/numbers/dots/dashes, and your Emby Connect username")
	}
	var n int
	e = m.db.QueryRow("SELECT COUNT(*) FROM access_requests WHERE email=? COLLATE NOCASE OR username=? COLLATE NOCASE", r.Email, r.Username).Scan(&n)
	if e != nil {
		return e
	}
	if n > 0 {
		return nil
	} // Same public response, no account enumeration.
	if e = m.db.QueryRow("SELECT COUNT(*) FROM access_requests WHERE status NOT IN ('active','declined')").Scan(&n); e != nil {
		return e
	}
	if n >= 500 {
		return errors.New("access requests are temporarily unavailable")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	res, e := m.db.Exec("INSERT INTO access_requests(name,email,username,connect_username,status,created_at,updated_at) VALUES(?,?,?,?,'pending',?,?)", r.Name, r.Email, r.Username, r.Connect, now, now)
	if e != nil {
		return e
	}
	id, e := res.LastInsertId()
	if e != nil {
		return e
	}
	m.notify(id, s, []notice{{r.Email, "Access request received", "Your request for access has been received and is awaiting administrator approval. No account has been created yet."}, {s.AdminEmail, "New Overmynd access request", r.Name + " (" + r.Email + ") requested access as " + r.Username + ". Review it in Overmynd Settings: " + s.PublicURL}})
	return nil
}

type notice struct{ to, subject, body string }

func (m *Manager) notify(id int64, s Settings, notices []notice) {
	failures := []string{}
	for _, n := range notices {
		if m.send(s, n.to, n.subject, n.body) != nil {
			failures = append(failures, "Email delivery failed for "+n.to)
		}
	}
	_, _ = m.db.Exec("UPDATE access_requests SET mail_error=? WHERE id=?", strings.Join(failures, "; "), id)
}
func (m *Manager) TestMail() error {
	s, e := m.Settings()
	if e != nil {
		return e
	}
	if !address(s.AdminEmail) || !address(s.From) {
		return errors.New("save valid sender and administrator email addresses first")
	}
	if e = m.send(s, s.AdminEmail, "Overmynd email test", "Email delivery from Overmynd is working."); e != nil {
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
	_, e := m.db.Exec("UPDATE access_requests SET status=?,error=?,updated_at=? WHERE id=?", state, message, time.Now().UTC().Format(time.RFC3339), id)
	return e
}
func (m *Manager) fail(id int64, s Settings, message string) error {
	if e := m.setState(id, "incomplete", message); e != nil {
		return e
	}
	m.notify(id, s, []notice{{s.AdminEmail, "Overmynd account setup incomplete", fmt.Sprintf("Access request %d needs attention: %s. Review it in Overmynd Settings.", id, message)}})
	return errors.New(message)
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
	res, e := m.db.Exec("UPDATE access_requests SET status='provisioning',error='',updated_at=? WHERE id=? AND status IN ('pending','incomplete')", time.Now().UTC().Format(time.RFC3339), id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
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
			return m.fail(id, s, "Unable to check existing Emby usernames")
		}
		for _, u := range users {
			if strings.EqualFold(u.Name, r.Username) {
				return m.fail(id, s, "Username already exists in Emby; no existing account was changed. Resolve the conflict in Emby before retrying")
			}
		}
		var created embyUser
		e = emby(ctx, s, "POST", "/Users/New", map[string]any{"Name": r.Username, "CopyFromUserId": s.TemplateID, "UserCopyOptions": []string{"UserPolicy", "UserConfiguration"}}, &created)
		if e != nil || !idPattern.MatchString(created.ID) {
			return m.fail(id, s, "Emby creation did not return a confirmed user ID. Check Emby for a partially created account before retrying")
		}
		r.EmbyID = created.ID
		createdNow = true
		if _, e = m.db.Exec("UPDATE access_requests SET emby_id=? WHERE id=?", r.EmbyID, id); e != nil {
			return e
		}
	}
	var user embyUser
	if e = emby(ctx, s, "GET", "/Users/"+url.PathEscape(r.EmbyID), nil, &user); e != nil {
		return m.fail(id, s, "Unable to verify the new Emby user")
	}
	if user.ID != r.EmbyID || !strings.EqualFold(user.Name, r.Username) {
		return m.fail(id, s, "Emby user identity changed; inspect it before retrying")
	}
	if user.Policy == nil {
		return m.fail(id, s, "Emby did not return the user policy")
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
	var link struct{ IsPending *bool }
	if e = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Connect/Link?ConnectUsername="+url.QueryEscape(r.Connect), nil, &link); e != nil || link.IsPending == nil {
		return m.fail(id, s, "Emby Connect linking failed; check the requested Connect username")
	}
	if e = m.setState(id, "awaiting_setup", ""); e != nil {
		return e
	}
	return m.sendSetup(r, s)
}
func (m *Manager) sendSetup(r Request, s Settings) error {
	token := secret()
	_, e := m.db.Exec("UPDATE access_requests SET token_hash=?,token_expires=?,updated_at=? WHERE id=?", hash(token), time.Now().Add(24*time.Hour).Unix(), time.Now().UTC().Format(time.RFC3339), r.ID)
	if e != nil {
		return e
	}
	m.notify(r.ID, s, []notice{{r.Email, "Your access request is approved", "Your Emby user has been created and remains disabled until setup is complete. Confirm any Emby Connect email first, then set your local password using this single-use link (expires in 24 hours):\n\n" + s.PublicURL + "/#access-setup=" + token + "\n\nUsername: " + r.Username + "\nDo not share this link. Overmynd will not ask for your Emby Connect password."}})
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
		m.notify(id, s, []notice{{r.Email, "Access request received", "Your access request is awaiting approval."}, {s.AdminEmail, "Access request awaiting approval", r.Name + " (" + r.Email + "): " + s.PublicURL}})
	case "declined":
		m.notify(id, s, []notice{{r.Email, "Access request update", "Your request for server access was not approved. Contact the server owner if you have questions."}})
	case "active":
		m.notify(id, s, []notice{completionNotice(r)})
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
	res, e := m.db.Exec("UPDATE access_requests SET status='declined',updated_at=? WHERE id=? AND status='pending'", time.Now().UTC().Format(time.RFC3339), id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("only pending requests can be declined")
	}
	r, e := m.get(id)
	if e != nil {
		return e
	}
	m.notify(id, s, []notice{{r.Email, "Access request update", "Your request for server access was not approved. Contact the server owner if you have questions."}})
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
	e := m.db.QueryRow("SELECT id FROM access_requests WHERE token_hash=? AND token_expires>? AND status='awaiting_setup'", hash(token), time.Now().Unix()).Scan(&id)
	if e != nil {
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
	res, e := m.db.Exec("UPDATE access_requests SET status='setting_up',updated_at=? WHERE id=? AND status='awaiting_setup'", time.Now().UTC().Format(time.RFC3339), id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
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
				m.notify(id, s, []notice{{s.AdminEmail, "Overmynd account setup needs attention", fmt.Sprintf("The recipient could not finish access request %d: %s. Check its status in Overmynd Settings: %s", id, recoveryMessage, s.PublicURL)}})
			}
		}
	}()
	var user embyUser
	if e = emby(ctx, s, "GET", "/Users/"+r.EmbyID, nil, &user); e != nil || user.Policy == nil {
		return errors.New("unable to read your Emby account")
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
		return errors.New("Emby could not set the password; retry or contact the server owner")
	}
	desiredPolicy["IsDisabled"] = false
	activationAttempted = true
	if e = emby(ctx, s, "POST", "/Users/"+r.EmbyID+"/Policy", desiredPolicy, nil); e != nil {
		return errors.New("unable to enable your account; contact the server owner")
	}
	if e = verifySettings(ctx, s, r, desiredPolicy, desiredConfig); e != nil {
		return e
	}
	_, e = m.db.Exec("UPDATE access_requests SET status='active',error='',token_hash='',token_expires=0,updated_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), id)
	if e != nil {
		return e
	}
	success = true
	m.notify(id, s, []notice{completionNotice(r)})
	return nil
}
