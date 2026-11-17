// Package playback withholds Emby streams from chosen libraries before the public
// dashboard shows them. Ported from Overmynd (OVERMYND_HIDDEN_EMBY_LIBRARIES).
package playback

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/tracearr"
)

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9-]+$`)

type Emby struct {
	URL string
	Key string
}

type library struct {
	ID     string `json:"Id"`
	ItemID string `json:"ItemId"`
}

func (e Emby) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(e.URL, "/")+path, nil)
	if err != nil {
		return errors.New("invalid media server URL")
	}
	req.Header.Set("X-Emby-Token", e.Key)
	req.Header.Set("Accept", "application/json")
	c := http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		return errors.New("media server connection failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return errors.New("media server returned an error")
	}
	return json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(out)
}

// Check proves the address and key work.
func (e Emby) Check(ctx context.Context) (name string, err error) {
	var info struct {
		ServerName string `json:"ServerName"`
	}
	if err := e.get(ctx, "/System/Info", &info); err != nil {
		return "", err
	}
	return info.ServerName, nil
}

// Filter drops Emby sessions that belong to an excluded library (comma-separated IDs).
// A session that cannot be placed in a library is withheld rather than made public.
// Sessions from other servers pass; ones with no server type are withheld.
func Filter(ctx context.Context, e Emby, sessions []tracearr.Session, excluded string) []tracearr.Session {
	blocked := map[string]bool{}
	for _, id := range strings.Split(excluded, ",") {
		if id = strings.TrimSpace(id); id != "" {
			blocked[id] = true
		}
	}
	if len(blocked) == 0 {
		return sessions
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out := make([]tracearr.Session, 0, len(sessions))
	var roots map[string]bool
	loaded := false
	for _, s := range sessions {
		if !strings.EqualFold(s.ServerType, "emby") {
			if s.ServerType != "" {
				out = append(out, s)
			}
			continue
		}
		if s.LibraryID != "" {
			if !blocked[s.LibraryID] {
				out = append(out, s)
			}
			continue
		}
		if e.URL == "" || e.Key == "" || !idPattern.MatchString(s.RatingKey) {
			continue
		}
		if !loaded {
			loaded = true
			var libs []library
			if e.get(ctx, "/Library/VirtualFolders", &libs) == nil {
				roots = map[string]bool{}
				for _, l := range libs {
					if l.ItemID != "" {
						roots[l.ItemID] = true
					}
				}
			}
		}
		if len(roots) == 0 {
			continue
		}
		var anc []library
		if e.get(ctx, "/Items/"+s.RatingKey+"/Ancestors", &anc) != nil {
			continue
		}
		identified, hidden := false, false
		for _, a := range anc {
			if blocked[a.ID] {
				hidden = true
			}
			if roots[a.ID] {
				identified = true
			}
		}
		if identified && !hidden {
			out = append(out, s)
		}
	}
	return out
}
