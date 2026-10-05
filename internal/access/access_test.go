package access

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/z354prance/overmynd/internal/database"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	m                *Manager
	policy           map[string]any
	templateDisabled bool
	existing         bool
	pending          bool
	linkFails        bool
	creates          int
	passwords        []string
	notices          []notice
	mailFails        bool
}

func testManager(t *testing.T) *fixture {
	t.Helper()
	db, e := database.Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	if e = db.Migrate(); e != nil {
		t.Fatal(e)
	}
	f := &fixture{m: New(db.DB), templateDisabled: true, policy: map[string]any{"IsDisabled": true, "IsAdministrator": false, "EnabledFolders": []string{"approved-library"}, "EnableAllFolders": false}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "test-key" {
			t.Error("missing API credential")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /Users/template":
			json.NewEncoder(w).Encode(embyUser{ID: "template", Policy: map[string]any{"IsDisabled": f.templateDisabled, "IsAdministrator": false}})
		case "GET /Users":
			if f.existing {
				json.NewEncoder(w).Encode([]embyUser{{ID: "existing", Name: "alice"}})
			} else {
				w.Write([]byte("[]"))
			}
		case "POST /Users/New":
			var body struct {
				Name, CopyFromUserId string
				UserCopyOptions      []string
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Name != "alice" || body.CopyFromUserId != "template" || len(body.UserCopyOptions) != 1 || body.UserCopyOptions[0] != "UserPolicy" {
				t.Error("unsafe user creation")
			}
			f.creates++
			json.NewEncoder(w).Encode(embyUser{ID: "created"})
		case "GET /Users/created":
			json.NewEncoder(w).Encode(embyUser{ID: "created", Name: "alice", Policy: f.policy})
		case "POST /Users/created/Policy":
			json.NewDecoder(r.Body).Decode(&f.policy)
		case "POST /Users/created/Password":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			f.passwords = append(f.passwords, body["NewPw"])
		case "POST /Users/created/Connect/Link":
			if r.URL.Query().Get("ConnectUsername") != "alice-connect" {
				t.Error("wrong Connect account")
			}
			if f.linkFails {
				http.Error(w, "no user", 400)
				return
			}
			json.NewEncoder(w).Encode(map[string]bool{"IsPending": f.pending})
		default:
			t.Errorf("unexpected Emby request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	f.m.send = func(s Settings, to, subject, body string) error {
		f.notices = append(f.notices, notice{to, subject, body})
		if f.mailFails {
			return errors.New("secret SMTP error")
		}
		return nil
	}
	if e = f.m.Save(Settings{Enabled: true, PublicURL: "https://overmynd.example", EmbyURL: server.URL, EmbyKey: "test-key", TemplateID: "template", SMTPHost: "smtp.example", SMTPPort: 465, SMTPSecurity: "tls", SMTPUser: "mailbox", SMTPPassword: "mail-secret", From: "access@example.com", AdminEmail: "admin@example.com"}); e != nil {
		t.Fatal(e)
	}
	return f
}
func (f *fixture) submit(t *testing.T) {
	t.Helper()
	if e := f.m.Submit(Request{Name: "Alice", Email: "alice@example.com", Username: "alice", Connect: "alice-connect"}); e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) token(t *testing.T) string {
	t.Helper()
	for i := len(f.notices) - 1; i >= 0; i-- {
		if _, token, ok := strings.Cut(f.notices[i].body, "#access-setup="); ok {
			return strings.Split(token, "\n")[0]
		}
	}
	t.Fatal("no setup token mailed")
	return ""
}
func TestApprovalAndOneTimeSetup(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if f.creates != 0 {
		t.Fatal("submission provisioned account")
	}
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	if f.creates != 1 || f.policy["IsDisabled"] != true || len(f.passwords) != 1 || len(f.passwords[0]) < 32 {
		t.Fatal("account not secured before setup")
	}
	if e := f.m.Approve(context.Background(), 1); e == nil {
		t.Fatal("double approval accepted")
	}
	token := f.token(t)
	var stored string
	f.m.db.QueryRow("SELECT token_hash FROM access_requests WHERE id=1").Scan(&stored)
	if stored == token || stored != hash(token) {
		t.Fatal("token not hashed")
	}
	if e := f.m.Setup(context.Background(), token, "a strong local password"); e != nil {
		t.Fatal(e)
	}
	if f.policy["IsDisabled"] != false || f.policy["IsAdministrator"] != false || f.policy["EnableAllFolders"] != false {
		t.Fatal("incorrect account policy")
	}
	if e := f.m.Setup(context.Background(), token, "another strong password"); e == nil {
		t.Fatal("replayed token")
	}
	r, _ := f.m.get(1)
	if r.Status != "active" {
		t.Fatal(r.Status)
	}
}
func TestSafetyAndRetry(t *testing.T) {
	t.Run("unsafe template", func(t *testing.T) {
		f := testManager(t)
		f.submit(t)
		f.templateDisabled = false
		if f.m.Approve(context.Background(), 1) == nil || f.creates != 0 {
			t.Fatal("unsafe template allowed")
		}
	})
	t.Run("existing username never adopted", func(t *testing.T) {
		f := testManager(t)
		f.submit(t)
		f.existing = true
		if f.m.Approve(context.Background(), 1) == nil || f.creates != 0 || len(f.passwords) != 0 {
			t.Fatal("existing account changed")
		}
	})
	t.Run("link retry reuses disabled user", func(t *testing.T) {
		f := testManager(t)
		f.submit(t)
		f.linkFails = true
		if f.m.Approve(context.Background(), 1) == nil {
			t.Fatal("expected failure")
		}
		r, _ := f.m.get(1)
		if r.Status != "incomplete" || r.EmbyID != "created" || f.policy["IsDisabled"] != true {
			t.Fatal(r)
		}
		f.linkFails = false
		if e := f.m.Approve(context.Background(), 1); e != nil {
			t.Fatal(e)
		}
		if f.creates != 1 {
			t.Fatal("duplicate user")
		}
	})
	t.Run("pending Connect remains disabled", func(t *testing.T) {
		f := testManager(t)
		f.submit(t)
		f.pending = true
		if e := f.m.Approve(context.Background(), 1); e != nil {
			t.Fatal(e)
		}
		token := f.token(t)
		if f.m.Setup(context.Background(), token, "a strong local password") == nil || f.policy["IsDisabled"] != true {
			t.Fatal("pending account enabled")
		}
		f.pending = false
		if e := f.m.Setup(context.Background(), token, "a strong local password"); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("expiry and resend", func(t *testing.T) {
		f := testManager(t)
		f.submit(t)
		f.m.Approve(context.Background(), 1)
		old := f.token(t)
		f.m.db.Exec("UPDATE access_requests SET token_expires=?", time.Now().Add(-time.Hour).Unix())
		if f.m.Setup(context.Background(), old, "a strong local password") == nil {
			t.Fatal("expired token accepted")
		}
		f.m.Resend(1)
		fresh := f.token(t)
		if old == fresh {
			t.Fatal("token not rotated")
		}
		if f.m.Setup(context.Background(), old, "a strong local password") == nil {
			t.Fatal("old token accepted")
		}
		if e := f.m.Setup(context.Background(), fresh, "a strong local password"); e != nil {
			t.Fatal(e)
		}
	})
	t.Run("mail failure recorded without credentials", func(t *testing.T) {
		f := testManager(t)
		f.mailFails = true
		f.submit(t)
		r, _ := f.m.get(1)
		if r.MailError == "" || strings.Contains(r.MailError, "secret") {
			t.Fatal(r.MailError)
		}
		f.mailFails = false
		f.m.Resend(1)
		r, _ = f.m.get(1)
		if r.MailError != "" {
			t.Fatal(r.MailError)
		}
	})
}
func TestDuplicateAndConcurrentRequests(t *testing.T) {
	f := testManager(t)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); f.submit(t) }()
	}
	wg.Wait()
	list, _ := f.m.List()
	if len(list) != 1 || len(f.notices) != 2 {
		t.Fatal("duplicate submission")
	}
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _ = f.m.Approve(context.Background(), 1) }()
	}
	wg.Wait()
	if f.creates != 1 {
		t.Fatal("concurrent duplicate provisioning")
	}
}
func TestSettingsAndRecovery(t *testing.T) {
	f := testManager(t)
	s, _ := f.m.Settings()
	redacted := Redact(s)
	b, _ := json.Marshal(redacted)
	if strings.Contains(string(b), "test-key") || strings.Contains(string(b), "mail-secret") {
		t.Fatal("secret leaked")
	}
	if e := f.m.Save(redacted); e != nil {
		t.Fatal(e)
	}
	s, _ = f.m.Settings()
	if s.EmbyKey != "test-key" || s.SMTPPassword != "mail-secret" {
		t.Fatal("blank replaced credentials")
	}
	f.submit(t)
	f.m.db.Exec("UPDATE access_requests SET status='provisioning',emby_id='created'")
	if e := RecoverInterrupted(f.m.db); e != nil {
		t.Fatal(e)
	}
	r, _ := f.m.get(1)
	if r.Status != "incomplete" || r.EmbyID != "created" {
		t.Fatal(r)
	}
	s.EmbyURL = "http://another-server"
	if f.m.Save(s) == nil {
		t.Fatal("server changed after provisioning")
	}
	f.m.db.Exec("UPDATE access_requests SET status='setting_up'")
	RecoverInterrupted(f.m.db)
	r, _ = f.m.get(1)
	if r.Status != "awaiting_setup" {
		t.Fatal(r.Status)
	}
}

func TestDeclineAndCorrection(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Correct(1, "new@example.com", "corrected-connect"); e != nil {
		t.Fatal(e)
	}
	r, _ := f.m.get(1)
	if r.Email != "new@example.com" || r.Connect != "corrected-connect" || r.Username != "alice" {
		t.Fatal(r)
	}
	if e := f.m.Decline(1); e != nil {
		t.Fatal(e)
	}
	if f.creates != 0 {
		t.Fatal("decline created a user")
	}
	if f.m.Approve(context.Background(), 1) == nil {
		t.Fatal("declined request approved")
	}
	if f.m.Correct(1, "another@example.com", "another-connect") == nil {
		t.Fatal("closed request changed")
	}
}
