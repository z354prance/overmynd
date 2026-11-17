package services

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
)

type Donations struct {
	Title       string `json:"title"`
	PayPalURL   string `json:"paypal_url"`
	CashAppURL  string `json:"cashapp_url"`
	CustomLabel string `json:"custom_label"`
	CustomURL   string `json:"custom_url"`
}

func (m *Manager) Donations() (Donations, error) {
	var d Donations
	var raw string
	err := m.db.DB.QueryRow("SELECT value FROM settings WHERE key='donations'").Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return d, nil
	}
	if err != nil {
		return d, err
	}
	err = json.Unmarshal([]byte(raw), &d)
	return d, err
}
func (m *Manager) SaveDonations(d Donations) error {
	if len(d.Title) > 80 || len(d.CustomLabel) > 40 {
		return errors.New("donation title or link label is too long")
	}
	for _, value := range []string{d.PayPalURL, d.CashAppURL, d.CustomURL} {
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(value) > 2048 || strings.ContainsAny(value, "\r\n") {
			return errors.New("donation links must be HTTPS URLs without embedded credentials")
		}
	}
	if d.CustomURL != "" && strings.TrimSpace(d.CustomLabel) == "" {
		return errors.New("enter a label for the custom donation link")
	}
	raw, err := json.Marshal(d)
	if err != nil {
		return err
	}
	_, err = m.db.DB.Exec("INSERT INTO settings(key,value,updated_at) VALUES('donations',?,datetime('now')) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at", string(raw))
	return err
}
