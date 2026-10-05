package access

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ResetDeleted is an explicit admin action, never automatic recovery from a
// public setup attempt. It invalidates all setup links and requires approval again.
func (m *Manager) ResetDeleted(ctx context.Context, id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	r, e := m.get(id)
	if e != nil {
		return errors.New("request not found")
	}
	if r.EmbyID == "" || (r.Status != "incomplete" && r.Status != "awaiting_setup" && r.Status != "active") {
		return errors.New("only a previously provisioned request can be reset after Emby deletion")
	}
	s, e := m.Settings()
	if e != nil {
		return e
	}
	var user embyUser
	e = emby(ctx, s, "GET", "/Users/"+url.PathEscape(r.EmbyID), nil, &user)
	if e == nil {
		return errors.New("the Emby user still exists; no request was reset")
	}
	var status embyStatusError
	if !errors.As(e, &status) || int(status) != http.StatusNotFound {
		return errors.New("unable to confirm the Emby user was deleted; check the connection and retry")
	}
	// A 404 alone might be a bad server route. Require a valid user list as well.
	var users []embyUser
	if e = emby(ctx, s, "GET", "/Users", nil, &users); e != nil || users == nil {
		return errors.New("unable to verify the Emby user list; no request was reset")
	}
	for _, u := range users {
		if u.ID == "" || u.Name == "" {
			return errors.New("Emby returned an incomplete user list; no request was reset")
		}
		if strings.EqualFold(u.ID, r.EmbyID) || strings.EqualFold(u.Name, r.Username) {
			return errors.New("the Emby user or username still exists; no request was reset")
		}
	}
	_, e = m.db.Exec("UPDATE access_requests SET status='pending',emby_id='',token_hash='',token_expires=0,error='',mail_error='',updated_at=? WHERE id=?", time.Now().UTC().Format(time.RFC3339), id)
	return e
}
