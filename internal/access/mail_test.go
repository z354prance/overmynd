package access

import (
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	netmail "net/mail"
	"strings"
	"testing"
)

func TestEmailsAreEscapedAndWellFormed(t *testing.T) {
	s := Settings{Provider: "emby", PublicURL: "https://status.example.com", From: "undermynd@example.com", AdminEmail: "a@example.com"}
	r := Request{ID: 1, Name: `<script>alert(1)</script> & "Co"`, Email: "x@example.com", Username: "u<b>", Connect: "c&d"}
	n := setupNotice(r, s, strings.Repeat("a", 64))
	raw, err := message(s, n.to, n.subject, n.l)
	if err != nil {
		t.Fatal(err)
	}
	m, err := netmail.ReadMessage(strings.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	mt, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mt != "multipart/alternative" {
		t.Fatalf("content type %q %v", mt, err)
	}
	mr := multipart.NewReader(m.Body, params["boundary"])
	var plain, htm string
	for {
		p, err := mr.NextPart()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(p)
		dec, _ := base64.StdEncoding.DecodeString(strings.ReplaceAll(string(b), "\r\n", ""))
		if strings.HasPrefix(p.Header.Get("Content-Type"), "text/html") {
			htm = string(dec)
		} else {
			plain = string(dec)
		}
	}
	link := "https://status.example.com/#access-setup=" + strings.Repeat("a", 64)
	if !strings.Contains(plain, link) || !strings.Contains(htm, `href="`+link+`"`) {
		t.Fatalf("setup link missing from a part")
	}
	if strings.Contains(htm, "<script>") || strings.Contains(htm, "<b>") {
		t.Fatalf("visitor text reached the HTML unescaped")
	}
	// Names appear in the greeting; the escaped form must be there instead.
	if !strings.Contains(htm, "&lt;script&gt;") {
		t.Fatalf("expected escaped name in HTML")
	}
	if m.Header.Get("Message-ID") == "" || m.Header.Get("Date") == "" {
		t.Fatal("missing Message-ID or Date")
	}
}

func TestLinksMustBeWebAddresses(t *testing.T) {
	if safeLink("javascript:alert(1)") != "" || safeLink("data:text/html,x") != "" {
		t.Fatal("unsafe link allowed")
	}
	if safeLink("https://example.com/a?b=1") == "" {
		t.Fatal("good link refused")
	}
}

func TestEveryLetterRenders(t *testing.T) {
	for _, s := range []Settings{{Provider: "emby", PublicURL: "https://x.example.com", From: "a@example.com", AdminEmail: "b@example.com"}, {Provider: "jellyfin", PublicURL: "https://x.example.com", ServerPublicURL: "https://jf.example.com", From: "a@example.com", AdminEmail: "b@example.com"}} {
		r := Request{ID: 2, Name: "Ann", Email: "ann@example.com", Username: "ann", Connect: "ann_c"}
		for _, n := range []notice{receiptNotice(r, s), adminNewNotice(r, s), adminPendingNotice(r, s), setupNotice(r, s, strings.Repeat("b", 64)), declinedNotice(r), adminProblemNotice(2, s, "x"), adminStuckNotice(2, s, "x"), testNotice(s), serverCompletionNotice(r, s)} {
			if n.to == "" || n.subject == "" || n.l.Title == "" || len(n.l.text()) < 40 || !strings.Contains(n.l.html(), "</html>") {
				t.Fatalf("bad letter %q", n.subject)
			}
		}
	}
}

func TestReadyEmailPointsToTheWebAddress(t *testing.T) {
	s := Settings{Provider: "emby", PublicURL: "https://status.example.com", ServerPublicURL: "https://emby.example.com", From: "a@example.com"}
	n := serverCompletionNotice(Request{Name: "Ann", Email: "ann@example.com", Username: "ann", Connect: "ann@connect.example.com"}, s)
	if !strings.Contains(n.l.text(), "https://emby.example.com") || !strings.Contains(n.l.html(), `href="https://emby.example.com"`) {
		t.Fatal("web address missing from the ready email")
	}
}
