package api

import (
	"strings"
	"testing"
)

func TestDonationSettings(t *testing.T) {
	h, token := testRouter(t)
	w := request(t, h, "", "PUT", "/api/v1/donations", `{"paypal_url":"https://paypal.me/example"}`)
	if w.Code != 401 {
		t.Fatal("public write accepted", w.Code)
	}
	for _, bad := range []string{"javascript:alert(1)", "http://example.com", "https://user:pass@example.com"} {
		w = request(t, h, token, "PUT", "/api/v1/donations", `{"paypal_url":"`+bad+`"}`)
		if w.Code != 400 {
			t.Fatal("unsafe link accepted", bad, w.Code)
		}
	}
	w = request(t, h, token, "PUT", "/api/v1/donations", `{"title":"Support us","paypal_url":"https://paypal.me/example"}`)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = request(t, h, "", "GET", "/api/v1/donations", "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), "https://paypal.me/example") {
		t.Fatal(w.Code, w.Body)
	}
}
