package access

import (
	"bufio"
	"context"
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
