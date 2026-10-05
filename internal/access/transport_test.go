package access

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEmbyDoesNotRedirectCredentials(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("followed credential-bearing redirect") }))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	if e := emby(context.Background(), Settings{EmbyURL: origin.URL, EmbyKey: "secret"}, "POST", "/Users/New", map[string]string{"Name": "alice"}, nil); e == nil {
		t.Fatal("redirect accepted")
	}
}
func TestSMTPRequiresStartTLSBeforeCredentials(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	done := make(chan []string, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			done <- nil
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(3 * time.Second))
		conn.Write([]byte("220 test ESMTP\r\n"))
		scanner := bufio.NewScanner(conn)
		var commands []string
		for scanner.Scan() {
			line := scanner.Text()
			commands = append(commands, line)
			if strings.HasPrefix(line, "EHLO") {
				conn.Write([]byte("250-test\r\n250 AUTH PLAIN\r\n"))
			} else if strings.HasPrefix(line, "STARTTLS") {
				conn.Write([]byte("502 TLS unavailable\r\n"))
			} else {
				conn.Write([]byte("500 rejected\r\n"))
			}
		}
		done <- commands
	}()
	host, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	e = sendMail(Settings{SMTPHost: host, SMTPPort: number, SMTPSecurity: "starttls", SMTPUser: "private-user", SMTPPassword: "private-password", From: "from@example.com"}, "to@example.com", "test", "body")
	if e == nil {
		t.Fatal("plaintext accepted")
	}
	for _, command := range <-done {
		if strings.HasPrefix(command, "AUTH") || strings.HasPrefix(command, "MAIL") {
			t.Fatal("sent credentials/mail without TLS")
		}
	}
}

func TestEmbyBoundedSafeRetry(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		status             int
		recover            bool
		calls              int
	}{
		{"read recovers", "GET", "/Users/alice", 503, true, 2},
		{"policy recovers", "POST", "/Users/alice/Policy", 502, true, 2},
		{"configuration recovers", "POST", "/Users/alice/Configuration", 500, true, 2},
		{"password recovers", "POST", "/Users/alice/Password", 504, true, 2},
		{"stops after retry", "GET", "/Users/alice", 503, false, 2},
		{"credentials need attention", "GET", "/Users/alice", 401, false, 1},
		{"no duplicate creation", "POST", "/Users/New", 503, false, 1},
		{"no duplicate invitation", "POST", "/Users/alice/Connect/Link", 503, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, _ := io.ReadAll(r.Body)
				if string(body) != `{"value":true}` {
					t.Errorf("retry changed body: %s", body)
				}
				if calls == 1 || !tc.recover {
					w.WriteHeader(tc.status)
					return
				}
				w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			var result map[string]bool
			err := emby(context.Background(), Settings{EmbyURL: server.URL}, tc.method, tc.path, map[string]bool{"value": true}, &result)
			if calls != tc.calls || (err == nil) != tc.recover {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if tc.recover && !result["ok"] {
				t.Fatal("missing response")
			}
		})
	}
}

func TestEmbyRetryRespectsCancellation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := emby(ctx, Settings{EmbyURL: server.URL}, "GET", "/Users/alice", nil, nil); err == nil {
		t.Fatal("expected cancellation")
	}
	if calls != 1 {
		t.Fatalf("retried after cancellation: %d", calls)
	}
}
