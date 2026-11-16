package access

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func testManager(t *testing.T, dir string, mails *[]string) *Manager {
	t.Helper()
	s := Settings{Provider: "emby", Enabled: true, PublicURL: "https://x.example.com", AdminEmail: "admin@example.com", From: "u@example.com"}
	m, err := New(dir, func() Settings { return s }, func(v Settings) error { s = v; return nil })
	if err != nil {
		t.Fatal(err)
	}
	m.send = func(_ Settings, to, subject string, _ letter) error {
		*mails = append(*mails, to+"|"+subject)
		return nil
	}
	return m
}

func TestSubmitDeclineAndPersistence(t *testing.T) {
	dir := t.TempDir()
	var mails []string
	m := testManager(t, dir, &mails)
	if err := m.Submit(Request{Name: "Ann", Email: "Ann@Example.com", Username: "ann", Connect: "ann@connect.example.com"}); err != nil {
		t.Fatal(err)
	}
	if len(mails) != 2 {
		t.Fatalf("want a receipt and an admin notice, got %v", mails)
	}
	// A repeat gets the same answer and creates nothing.
	if err := m.Submit(Request{Name: "Ann", Email: "ann@example.com", Username: "other", Connect: "x@connect.example.com"}); err != nil {
		t.Fatal(err)
	}
	if l, _ := m.List(); len(l) != 1 {
		t.Fatalf("duplicate was stored: %v", l)
	}
	if err := m.Submit(Request{Name: "B", Email: "bad", Username: "bob", Connect: "b@connect.example.com"}); err == nil {
		t.Fatal("bad email accepted")
	}
	if err := m.Decline(1); err != nil {
		t.Fatal(err)
	}
	if err := m.Decline(1); err == nil {
		t.Fatal("declined twice")
	}
	m2 := testManager(t, dir, &mails)
	l, _ := m2.List()
	if len(l) != 1 || l[0].Status != "declined" || l[0].Email != "ann@example.com" {
		t.Fatalf("not persisted: %+v", l)
	}
	if err := m2.Submit(Request{Name: "C", Email: "c@example.com", Username: "carl", Connect: "c@connect.example.com"}); err != nil {
		t.Fatal(err)
	}
	if l, _ := m2.List(); l[0].ID != 2 && l[len(l)-1].ID != 2 {
		t.Fatalf("ids reused: %+v", l)
	}
}

func TestRecoverAndSetupLinkIsChecked(t *testing.T) {
	var mails []string
	m := testManager(t, t.TempDir(), &mails)
	_ = m.Submit(Request{Name: "Ann", Email: "ann@example.com", Username: "ann", Connect: "c@connect.example.com"})
	_ = m.update(1, func(x *rec) { x.Status = "setting_up" })
	if err := m.RecoverInterrupted(); err != nil {
		t.Fatal(err)
	}
	r, _ := m.get(1)
	if r.Status != "awaiting_setup" || r.Error == "" {
		t.Fatalf("not recovered: %+v", r)
	}
	if err := m.Setup(context.Background(), strings.Repeat("a", 64), "a long enough password"); err == nil {
		t.Fatal("unknown token accepted")
	}
	if err := m.Setup(context.Background(), "short", "x"); err == nil {
		t.Fatal("malformed setup accepted")
	}
	_ = filepath.Join
}

func TestDisabledRefusesRequests(t *testing.T) {
	var mails []string
	m := testManager(t, t.TempDir(), &mails)
	s, _ := m.Settings()
	s.Enabled = false
	m.Load = func() Settings { return s }
	if err := m.Submit(Request{Name: "A", Email: "a@example.com", Username: "abc", Connect: "c@connect.example.com"}); err == nil {
		t.Fatal("accepted while disabled")
	}
}

func TestEmbyConnectMustBeAnEmail(t *testing.T) {
	var mails []string
	m := testManager(t, t.TempDir(), &mails)
	if err := m.Submit(Request{Name: "A", Email: "a@example.com", Username: "abc", Connect: "just_a_username"}); err == nil {
		t.Fatal("a Connect name that is not an email was accepted")
	}
	if err := m.Submit(Request{Name: "A", Email: "a@example.com", Username: "abc", Connect: "A@Connect.Example.com"}); err != nil {
		t.Fatal(err)
	}
	if l, _ := m.List(); len(l) != 1 || l[0].Connect != "a@connect.example.com" {
		t.Fatalf("not stored lower-cased: %+v", l)
	}
	if err := m.Correct(1, "a@example.com", "not an email"); err == nil {
		t.Fatal("correction accepted a non-email")
	}
}
