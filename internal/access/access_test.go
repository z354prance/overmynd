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
	refuseDisable      bool
	templatePolicy     map[string]any
	templateConfig     map[string]any
	config             map[string]any
	ignoreRestrictions bool
	broadenOnEnable    bool
	bornEnabled        bool
	configFails        bool
	m                  *Manager
	policy             map[string]any
	templateDisabled   bool
	existing           bool
	pending            bool
	linkFails          bool
	linkCalls          int
	connectName        string
	connectType        string
	pendingJSON        string
	pendingHTTP        int
	deleted            bool
	usersHTTP          int
	creates            int
	passwords          []string
	notices            []notice
	mailFails          bool
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
	f := &fixture{m: New(db.DB), templateDisabled: true, connectName: "alice-connect", connectType: "LinkedUser", policy: map[string]any{"IsDisabled": true, "IsAdministrator": false, "EnabledFolders": []string{"approved-library"}, "EnableAllFolders": false}}
	f.templatePolicy = map[string]any{"IsDisabled": true, "IsAdministrator": false, "EnableAllFolders": false, "EnabledFolders": []string{"approved-library"}, "EnableContentDeletion": false, "EnableMediaPlayback": true, "EnableRemoteAccess": true, "EnableAllChannels": false, "EnabledChannels": []string{}, "SimultaneousStreamLimit": 2, "RemoteClientBitrateLimit": 8000000, "EnableVideoPlaybackTranscoding": false}
	f.templateConfig = map[string]any{"AudioLanguagePreference": "eng", "SubtitleMode": "OnlyForced", "ProfilePin": "1234"}
	f.config = map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Emby-Token") != "test-key" {
			t.Error("missing API credential")
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("missing JSON response negotiation")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case "GET /Users/template":
			policy := copySettings(f.templatePolicy)
			policy["IsDisabled"] = f.templateDisabled
			json.NewEncoder(w).Encode(embyUser{ID: "template", Policy: policy, Configuration: f.templateConfig})
		case "GET /Users":
			if f.usersHTTP != 0 {
				http.Error(w, "failed", f.usersHTTP)
				return
			}
			if f.existing {
				json.NewEncoder(w).Encode([]embyUser{{ID: "existing", Name: "alice"}})
			} else if f.creates > 0 && !f.deleted {
				json.NewEncoder(w).Encode([]embyUser{{ID: "created", Name: "alice"}})
			} else {
				w.Write([]byte("[]"))
			}
		case "POST /Users/New":
			var body struct {
				Name, CopyFromUserId string
				UserCopyOptions      []string
			}
			json.NewDecoder(r.Body).Decode(&body)
			if body.Name != "alice" || body.CopyFromUserId != "template" || len(body.UserCopyOptions) != 2 || body.UserCopyOptions[0] != "UserPolicy" || body.UserCopyOptions[1] != "UserConfiguration" {
				t.Error("unsafe user creation")
			}
			// Simulate a server that copies the disabled flag but leaves broad defaults.
			f.policy = map[string]any{"IsDisabled": !f.bornEnabled, "IsAdministrator": false, "EnableAllFolders": true, "EnabledFolders": []string{}, "EnableContentDeletion": true}
			f.creates++
			f.deleted = false
			json.NewEncoder(w).Encode(embyUser{ID: "created"})
		case "GET /Users/created":
			if f.deleted {
				http.NotFound(w, r)
				return
			}
			json.NewEncoder(w).Encode(embyUser{ID: "created", Name: "alice", Policy: f.policy, ConnectUserName: f.connectName, ConnectLinkType: f.connectType, Configuration: f.config})
		case "POST /Users/created/Policy":
			json.NewDecoder(r.Body).Decode(&f.policy)
			if f.refuseDisable {
				f.policy["IsDisabled"] = false
			}
			if f.ignoreRestrictions || (f.broadenOnEnable && f.policy["IsDisabled"] == false) {
				f.policy["EnableAllFolders"] = true
			}
		case "POST /Users/created/Configuration":
			if f.configFails {
				http.Error(w, "failed", 500)
				return
			}
			json.NewDecoder(r.Body).Decode(&f.config)
		case "POST /Users/created/Password":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			f.passwords = append(f.passwords, body["NewPw"])
		case "GET /Connect/Pending":
			if f.pendingHTTP != 0 {
				http.Error(w, "upstream error", f.pendingHTTP)
				return
			}
			if f.pendingJSON != "" {
				w.Write([]byte(f.pendingJSON))
				return
			}
			if f.pending {
				w.Write([]byte(`[{"LocalUserId":"created"}]`))
			} else {
				w.Write([]byte(`[]`))
			}
		case "POST /Users/created/Connect/Link":
			f.linkCalls++
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

func TestSetupReadsExistingConnectLinkWithoutRelinking(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	// A second link-creation request would fail on the real server.
	f.linkFails = true
	if e := f.m.Setup(context.Background(), f.token(t), "a strong local password"); e != nil {
		t.Fatal(e)
	}
	if f.linkCalls != 1 {
		t.Fatalf("created Connect link %d times", f.linkCalls)
	}
}
func TestConnectVerificationFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, connectName, connectType, pending string
		status                                  int
	}{
		{name: "unlinked", connectType: ""},
		{name: "guest", connectName: "alice-connect", connectType: "Guest"},
		{name: "different identity", connectName: "someone-else", connectType: "LinkedUser"},
		{name: "null pending list", connectName: "alice-connect", connectType: "LinkedUser", pending: "null"},
		{name: "malformed pending list", connectName: "alice-connect", connectType: "LinkedUser", pending: "{}"},
		{name: "unidentified pending entry", connectName: "alice-connect", connectType: "LinkedUser", pending: `[{"Unknown":"value"}]`},
		{name: "pending API failure", connectName: "alice-connect", connectType: "LinkedUser", status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := testManager(t)
			f.submit(t)
			if e := f.m.Approve(context.Background(), 1); e != nil {
				t.Fatal(e)
			}
			f.connectName = tc.connectName
			f.connectType = tc.connectType
			f.pendingJSON = tc.pending
			f.pendingHTTP = tc.status
			token := f.token(t)
			if f.m.Setup(context.Background(), token, "a strong local password") == nil {
				t.Fatal("unsafe setup accepted")
			}
			if len(f.passwords) != 1 || f.policy["IsDisabled"] != true || f.linkCalls != 1 {
				t.Fatal("verification changed account")
			}
			r, _ := f.m.get(1)
			if r.Status != "awaiting_setup" {
				t.Fatal(r.Status)
			}
		})
	}
}
func TestConnectVerificationIgnoresIdentifiedOtherPendingUser(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	f.pendingJSON = `[{"LocalUserId":"another-user"}]`
	f.connectName = "ALICE-CONNECT"
	if e := f.m.Setup(context.Background(), f.token(t), "a strong local password"); e != nil {
		t.Fatal(e)
	}
}

func TestResetAfterDeletion(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	oldToken := f.token(t)
	if f.m.ResetDeleted(context.Background(), 1) == nil {
		t.Fatal("reset existing Emby user")
	}
	f.deleted = true
	// Public setup must not recreate deleted users.
	if f.m.Setup(context.Background(), oldToken, "a strong local password") == nil || f.creates != 1 {
		t.Fatal("setup recreated deleted account")
	}
	if e := f.m.ResetDeleted(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	r, _ := f.m.get(1)
	if r.Status != "pending" || r.EmbyID != "" {
		t.Fatal(r)
	}
	if f.m.Setup(context.Background(), oldToken, "a strong local password") == nil {
		t.Fatal("old token survived reset")
	}
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	if f.creates != 2 {
		t.Fatal("fresh approval did not create user")
	}
	if e := f.m.Setup(context.Background(), f.token(t), "a strong local password"); e != nil {
		t.Fatal(e)
	}
}
func TestResetRequiresConfirmedAbsence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		http     int
		existing bool
	}{{"list failure", 503, false}, {"username reused", 0, true}} {
		t.Run(tc.name, func(t *testing.T) {
			f := testManager(t)
			f.submit(t)
			if e := f.m.Approve(context.Background(), 1); e != nil {
				t.Fatal(e)
			}
			f.deleted = true
			f.usersHTTP = tc.http
			f.existing = tc.existing
			if f.m.ResetDeleted(context.Background(), 1) == nil {
				t.Fatal("uncertain deletion accepted")
			}
			r, _ := f.m.get(1)
			if r.EmbyID != "created" || r.Status != "awaiting_setup" {
				t.Fatal("changed uncertain request")
			}
		})
	}
}

func TestExplicitTemplatePermissionsAndConfiguration(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	policy, config := templateSettings(embyUser{Policy: f.templatePolicy, Configuration: f.templateConfig}, true)
	if diff := settingsDifference(policy, f.policy, true); diff != "" {
		t.Fatal("template policy mismatch", diff)
	}
	if diff := settingsDifference(config, f.config, false); diff != "" {
		t.Fatal("template config mismatch", diff)
	}
	if f.config["ProfilePin"] != "" {
		t.Fatal("copied template PIN")
	}
	// Simulate the old bug or permissions changed while awaiting setup.
	f.policy["EnableAllFolders"] = true
	f.policy["EnableContentDeletion"] = true
	if e := f.m.Setup(context.Background(), f.token(t), "a strong local password"); e != nil {
		t.Fatal(e)
	}
	policy["IsDisabled"] = false
	if diff := settingsDifference(policy, f.policy, true); diff != "" {
		t.Fatal("setup kept broad permissions", diff)
	}
}
func TestPermissionFailuresKeepAccountDisabled(t *testing.T) {
	for _, mode := range []string{"ignored restrictions", "configuration failure", "enabled on creation"} {
		t.Run(mode, func(t *testing.T) {
			f := testManager(t)
			f.submit(t)
			switch mode {
			case "ignored restrictions":
				f.ignoreRestrictions = true
			case "configuration failure":
				f.configFails = true
			case "enabled on creation":
				f.bornEnabled = true
			}
			if f.m.Approve(context.Background(), 1) == nil {
				t.Fatal("unsafe provisioning succeeded")
			}
			if f.policy["IsDisabled"] != true || f.linkCalls != 0 {
				t.Fatal("unsafe account linked or enabled")
			}
			r, _ := f.m.get(1)
			if r.Status != "incomplete" {
				t.Fatal(r.Status)
			}
			var token string
			f.m.db.QueryRow("SELECT token_hash FROM access_requests WHERE id=1").Scan(&token)
			if token != "" {
				t.Fatal("setup token issued despite failure")
			}
		})
	}
}
func TestActivationReadbackFailureDisablesAccount(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	f.broadenOnEnable = true
	if f.m.Setup(context.Background(), f.token(t), "a strong local password") == nil {
		t.Fatal("activation mismatch accepted")
	}
	if f.policy["IsDisabled"] != true {
		t.Fatal("account left enabled after mismatch")
	}
	r, _ := f.m.get(1)
	if r.Status == "active" {
		t.Fatal("marked active despite mismatch")
	}
}
func TestReapplyTemplateRepairsExistingAccount(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	oldToken := f.token(t)
	if e := f.m.Setup(context.Background(), oldToken, "a strong local password"); e != nil {
		t.Fatal(e)
	}
	f.policy["EnableAllFolders"] = true
	if e := f.m.ReapplyTemplate(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	if f.creates != 1 || f.linkCalls != 1 || f.policy["IsDisabled"] != true || f.policy["EnableAllFolders"] != false {
		t.Fatal("repair failed or recreated account/link")
	}
	if f.m.Setup(context.Background(), oldToken, "a strong local password") == nil {
		t.Fatal("old setup link usable")
	}
	if e := f.m.Setup(context.Background(), f.token(t), "a new strong local password"); e != nil {
		t.Fatal(e)
	}
	if f.policy["EnableAllFolders"] != false {
		t.Fatal("repair did not preserve restrictions")
	}
}

func TestCannotDisableIsReportedWithoutIssuingSetup(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	f.bornEnabled = true
	f.refuseDisable = true
	err := f.m.Approve(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "manually") {
		t.Fatal("failed to report manual disable requirement", err)
	}
	if f.linkCalls != 0 {
		t.Fatal("linked unsafe user")
	}
	r, _ := f.m.get(1)
	if !strings.Contains(r.Error, "manually") {
		t.Fatal(r.Error)
	}
}
func TestReapplyDoesNotModifyPromotedAdmin(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if e := f.m.Approve(context.Background(), 1); e != nil {
		t.Fatal(e)
	}
	f.policy["IsAdministrator"] = true
	if f.m.ReapplyTemplate(context.Background(), 1) == nil {
		t.Fatal("modified administrator")
	}
	if f.policy["IsAdministrator"] != true {
		t.Fatal("changed promoted account")
	}
}
