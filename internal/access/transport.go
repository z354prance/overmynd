package access

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

type embyUser struct {
	ID              string         `json:"Id"`
	Name            string         `json:"Name"`
	Policy          map[string]any `json:"Policy"`
	ConnectUserName string         `json:"ConnectUserName,omitempty"`
	ConnectLinkType string         `json:"ConnectLinkType,omitempty"`
}

type embyStatusError int

func (e embyStatusError) Error() string { return fmt.Sprintf("Emby returned HTTP %d", int(e)) }

func emby(ctx context.Context, s Settings, method, path string, body, out any) error {
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(s.EmbyURL, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return errors.New("invalid Emby URL")
	}
	req.Header.Set("X-Emby-Token", s.EmbyKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Emby connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return embyStatusError(res.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
	}
	return nil
}
func sendMail(s Settings, to, subject, body string) error {
	if !address(to) || !address(s.From) || strings.ContainsAny(subject, "\r\n") || s.SMTPHost == "" || s.SMTPPort < 1 || s.SMTPPort > 65535 {
		return errors.New("invalid email settings")
	}
	host := net.JoinHostPort(s.SMTPHost, strconv.Itoa(s.SMTPPort))
	config := &tls.Config{ServerName: s.SMTPHost, MinVersion: tls.VersionTLS12}
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	var conn net.Conn
	var err error
	if s.SMTPSecurity == "tls" {
		conn, err = tls.DialWithDialer(dialer, "tcp", host, config)
	} else if s.SMTPSecurity == "starttls" {
		conn, err = dialer.Dial("tcp", host)
	} else {
		return errors.New("TLS or STARTTLS is required")
	}
	if err != nil {
		return err
	}
	defer conn.Close()
	if err = conn.SetDeadline(time.Now().Add(20 * time.Second)); err != nil {
		return err
	}
	client, err := smtp.NewClient(conn, s.SMTPHost)
	if err != nil {
		return err
	}
	defer client.Close()
	if s.SMTPSecurity == "starttls" {
		if err = client.StartTLS(config); err != nil {
			return err
		}
	}
	if s.SMTPUser != "" {
		if err = client.Auth(smtp.PlainAuth("", s.SMTPUser, s.SMTPPassword, s.SMTPHost)); err != nil {
			return err
		}
	}
	if err = client.Mail(s.From); err != nil {
		return err
	}
	if err = client.Rcpt(to); err != nil {
		return err
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	message := "From: " + s.From + "\r\nTo: " + to + "\r\nSubject: " + subject + "\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n") + "\r\n"
	if _, err = io.WriteString(writer, message); err != nil {
		return err
	}
	if err = writer.Close(); err != nil {
		return err
	}
	return client.Quit()
}
