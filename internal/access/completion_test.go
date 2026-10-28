package access

import (
	"context"
	"strings"
	"testing"
)

func TestCompletionEmailOnlyAfterVerifiedSetupAndCanResend(t *testing.T) {
	f := testManager(t)
	f.submit(t)
	if err := f.m.Approve(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	count := func() int {
		n := 0
		for _, mail := range f.notices {
			if strings.HasPrefix(mail.subject, "Emby setup complete") {
				n++
			}
		}
		return n
	}
	if count() != 0 {
		t.Fatal("completion sent before setup")
	}
	token := f.token(t)
	f.pending = true
	if err := f.m.Setup(context.Background(), token, "a strong local password"); err == nil {
		t.Fatal("pending link accepted")
	}
	if count() != 0 {
		t.Fatal("completion sent after failed setup")
	}
	f.pending = false
	if err := f.m.Setup(context.Background(), token, "a strong local password"); err != nil {
		t.Fatal(err)
	}
	if count() != 1 {
		t.Fatal("missing completion email")
	}
	mail := f.notices[len(f.notices)-1]
	if mail.to != "alice@example.com" {
		t.Fatal("wrong recipient")
	}
	for _, required := range []string{"alice-connect", "username is alice", "TVs and streaming devices", "Web browser", "Phones and tablets", "Gaming consoles", "https://app.emby.media", "https://tv.emby.media", "https://emby.media/download.html"} {
		if !strings.Contains(mail.body, required) {
			t.Fatalf("missing %s", required)
		}
	}
	for _, secret := range []string{token, "a strong local password", "test-key", "mail-secret"} {
		if strings.Contains(mail.body, secret) {
			t.Fatal("completion email contains a secret")
		}
	}
	if err := f.m.Setup(context.Background(), token, "a strong local password"); err == nil {
		t.Fatal("token replay accepted")
	}
	if count() != 1 {
		t.Fatal("duplicate completion on replay")
	}
	if err := f.m.Resend(1); err != nil {
		t.Fatal(err)
	}
	if count() != 2 || f.notices[len(f.notices)-1] != mail {
		t.Fatal("resend differs from completion")
	}
}
