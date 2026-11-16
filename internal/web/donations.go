package web

import (
	"errors"
	"net/url"
	"strings"

	"github.com/z354prance/undermynd/internal/config"
)

// validDonations cleans and checks the public donation links: HTTPS only, no
// credentials, no line breaks. Blank links are fine and hide the button.
func validDonations(d *config.Donations) error {
	d.Title, d.CustomLabel = strings.TrimSpace(d.Title), strings.TrimSpace(d.CustomLabel)
	d.PayPalURL, d.CashAppURL, d.CustomURL = strings.TrimSpace(d.PayPalURL), strings.TrimSpace(d.CashAppURL), strings.TrimSpace(d.CustomURL)
	if len(d.Title) > 80 || len(d.CustomLabel) > 40 {
		return errors.New("the donation title or link label is too long")
	}
	for _, v := range []string{d.PayPalURL, d.CashAppURL, d.CustomURL} {
		if v == "" {
			continue
		}
		u, err := url.Parse(v)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(v) > 2048 || strings.ContainsAny(v, "\r\n") {
			return errors.New("donation links must be HTTPS URLs without embedded credentials")
		}
	}
	if d.CustomURL != "" && d.CustomLabel == "" {
		return errors.New("enter a label for the custom donation link")
	}
	return nil
}
