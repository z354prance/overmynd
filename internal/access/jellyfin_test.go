package access

import (
	"context"
	"strings"
	"testing"
)

func jellyfinManager(t *testing.T) *fixture {
	t.Helper()
	f := testManager(t)
	f.jellyfin = true
	f.bornEnabled = true
	delete(f.templateConfig, "ProfilePin")
	s, _ := f.m.Settings()
	s.Provider = "jellyfin"
	s.ServerPublicURL = "https://watch.example.com"
	if err := f.m.Save(s); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestJellyfinApprovalSetupAndResend(t *testing.T) {
	f := jellyfinManager(t)
	if err := f.m.Submit(Request{Name: "Alice", Email: "alice@example.com", Username: "alice"}); err != nil {
		t.Fatal(err)
	}
	if f.creates != 0 {
		t.Fatal("created before approval")
	}
	if err := f.m.Approve(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if f.policy["IsDisabled"] != true || f.policy["EnableAllFolders"] != false || f.linkCalls != 0 {
		t.Fatal("unsafe provisioning")
	}
	token := f.token(t)
	if err := f.m.Setup(context.Background(), token, "a strong jellyfin password"); err != nil {
		t.Fatal(err)
	}
	if f.policy["IsDisabled"] != false || f.policy["EnableAllFolders"] != false {
		t.Fatal("lost template restrictions")
	}
	if err := f.m.Setup(context.Background(), token, "a strong jellyfin password"); err == nil {
		t.Fatal("setup replay accepted")
	}
	if err := f.m.Resend(1); err != nil {
		t.Fatal(err)
	}
	mail := f.notices[len(f.notices)-1]
	for _, want := range []string{"Jellyfin", "https://watch.example.com", "alice", "Quick Connect"} {
		if !strings.Contains(mail.body, want) {
			t.Fatal("missing instructions", want)
		}
	}
	for _, secret := range []string{token, "a strong jellyfin password", "test-key", "Emby Connect"} {
		if strings.Contains(mail.body, secret) {
			t.Fatal("unexpected completion content")
		}
	}
	s, _ := f.m.Settings()
	s.Provider = "emby"
	if err := f.m.Save(s); err == nil {
		t.Fatal("provider changed under existing account")
	}
}

func TestJellyfinRestrictionFailureKeepsDisabled(t *testing.T) {
	f := jellyfinManager(t)
	f.submit(t)
	f.ignoreRestrictions = true
	if err := f.m.Approve(context.Background(), 1); err == nil {
		t.Fatal("accepted broadened permissions")
	}
	if f.policy["IsDisabled"] != true {
		t.Fatal("failed provision left enabled")
	}
	for _, mail := range f.notices {
		if strings.Contains(mail.body, "#access-setup=") {
			t.Fatal("mailed unsafe setup")
		}
	}
}
