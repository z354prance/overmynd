package web

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/playback"
	"github.com/z354prance/overmynd/internal/tracearr"
)

// Signed poster links expire with the process, as in Overmynd.
var posterKey = func() []byte { b := make([]byte, 32); rand.Read(b); return b }()

func posterSignature(path string) string {
	h := hmac.New(sha256.New, posterKey)
	h.Write([]byte(path))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func validPosterPath(path string) bool {
	u, err := url.Parse(path)
	return err == nil && !u.IsAbs() && u.Host == "" && u.User == nil && u.Fragment == "" &&
		u.Path == "/api/v1/images/proxy" && u.RawPath == "" && len(path) < 4096
}

func signedPoster(path string) string {
	if path == "" || !validPosterPath(path) {
		return ""
	}
	return "/api/v1/playback/poster?" + url.Values{"path": {path}, "signature": {posterSignature(path)}}.Encode()
}

var coverPath = regexp.MustCompile(`^/MediaCover/([1-9][0-9]*/poster|Albums/[1-9][0-9]*/cover)\.jpg$`)

func coverSignature(arr, path string) string {
	h := hmac.New(sha256.New, posterKey)
	h.Write([]byte("arr\n" + arr + "\n" + path))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// signedCover links an arr's own poster through this server, so the arr key never leaves it.
func signedCover(arr, path string) string {
	if arr == "" || !coverPath.MatchString(path) {
		return ""
	}
	return "/api/v1/playback/poster?" + url.Values{"arr": {arr}, "path": {path}, "signature": {coverSignature(arr, path)}}.Encode()
}

func (s *Server) tracearr() (*tracearr.Client, bool) {
	p := s.Cfg.Get().Playback
	if p.TracearrURL == "" || p.TracearrKey == "" {
		return nil, false
	}
	return tracearr.New(p.TracearrURL, string(p.TracearrKey)), true
}

// One poll serves every viewer for a few seconds, and a slow or dead Tracearr
// can never hold the page up for long.
var pbCache struct {
	sync.Mutex
	at     time.Time
	key    string
	body   []byte
	status string
}

func (s *Server) playbackJSON() []byte {
	pbCache.Lock()
	defer pbCache.Unlock()
	p := s.Cfg.Get().Playback
	key := p.TracearrURL + "|" + p.EmbyURL + "|" + p.HiddenLibraries
	if pbCache.body != nil && pbCache.key == key && time.Since(pbCache.at) < 4*time.Second {
		return pbCache.body
	}
	out := func(sessions []tracearr.Session, errs []string, status string) []byte {
		if sessions == nil {
			sessions = []tracearr.Session{}
		}
		if errs == nil {
			errs = []string{}
		}
		b, _ := json.Marshal(map[string]any{"sessions": sessions, "errors": errs})
		pbCache.at, pbCache.key, pbCache.body, pbCache.status = time.Now(), key, b, status
		return b
	}
	c, ok := s.tracearr()
	if !ok {
		return out(nil, nil, "Tracearr is not set up")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sessions, err := c.Streams(ctx)
	if err != nil {
		s.Log.Printf("playback: %v", err)
		return out(nil, []string{"Now playing is unavailable from Tracearr."}, "Tracearr: "+err.Error())
	}
	total := len(sessions)
	sessions = playback.Filter(ctx, playback.Emby{URL: p.EmbyURL, Key: string(p.EmbyKey)}, sessions, p.HiddenLibraries)
	for i := range sessions {
		sessions[i].PosterURL = signedPoster(sessions[i].PosterURL)
	}
	return out(sessions, nil, fmt.Sprintf("Tracearr reports %d stream(s); %d shown, %d hidden", total, len(sessions), total-len(sessions)))
}

func (s *Server) v1Playback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(s.playbackJSON())
}

// playbackStatus tells the admin what the last poll saw, and whether posters load.
func (s *Server) playbackStatus(w http.ResponseWriter, r *http.Request) {
	s.playbackJSON()
	pbCache.Lock()
	st := pbCache.status
	pbCache.Unlock()
	if c, ok := s.tracearr(); ok {
		ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
		defer cancel()
		if sessions, err := c.Streams(ctx); err == nil {
			withArt, sample := 0, ""
			for _, x := range sessions {
				if x.PosterURL != "" {
					withArt++
					if sample == "" {
						sample = x.PosterURL
					}
				}
			}
			st += fmt.Sprintf(" · %d of %d have a poster link from Tracearr", withArt, len(sessions))
			if sample != "" {
				st += " · " + s.checkPoster(ctx, sample)
			}
		}
		if rec := c.RecentlyAdded(ctx); len(rec.Items) > 0 {
			withArt, sample := 0, ""
			for _, it := range rec.Items {
				if it.PosterURL != "" {
					withArt++
					if sample == "" {
						sample = it.PosterURL
					}
				}
			}
			st += fmt.Sprintf(" · recently added: %d of %d have a poster link", withArt, len(rec.Items))
			if sample != "" {
				st += " · " + s.checkPoster(ctx, sample)
			}
		}
	}
	writeJSON(w, map[string]string{"status": st})
}

// checkPoster fetches one poster the way the proxy would and says what came back.
func (s *Server) checkPoster(ctx context.Context, path string) string {
	if !validPosterPath(path) {
		return "poster link rejected by the proxy (" + path + ")"
	}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(s.Cfg.Get().Playback.TracearrURL, "/")+path, nil)
	if err != nil {
		return "poster check failed"
	}
	resp, err := (&http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}).Do(req)
	if err != nil {
		return "poster fetch failed: " + err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return fmt.Sprintf("poster fetch: HTTP %d, %s, %d bytes", resp.StatusCode, http.DetectContentType(body), len(body))
}

// arrRecent turns the arrs' import history into Recently Added items.
func (s *Server) arrRecent(musicOnly bool) []map[string]any {
	items := []map[string]any{}
	if s.Progress == nil {
		return items
	}
	for i, x := range s.Progress.Snapshot().Recent {
		isMusic := x.Kind != "movie" && x.Kind != "episode"
		if musicOnly && !isMusic {
			continue
		}
		it := map[string]any{"id": "recent:" + itoa(i) + ":" + x.Title, "title": x.Title, "added_at": x.Date, "stage": "available"}
		switch x.Kind {
		case "episode":
			it["kind"], it["show_title"], it["episodes"] = "series", x.Title, x.Episodes
		case "movie":
			it["kind"] = "movie"
			if x.Year != 0 {
				it["year"] = x.Year
			}
		default:
			it["kind"] = x.Kind
		}
		if u := signedCover(x.Arr, x.Poster); u != "" {
			it["poster_url"] = u
		}
		items = append(items, it)
	}
	return items
}

// v1Recent shows Tracearr's confirmed movie and show additions with music from
// Lidarr merged in; without Tracearr it falls back to the arrs' import history.
func (s *Server) v1Recent(w http.ResponseWriter, r *http.Request) {
	c, ok := s.tracearr()
	if !ok {
		items := s.arrRecent(false)
		if len(items) > 6 {
			items = items[:6]
		}
		writeJSON(w, map[string]any{"items": items, "errors": []string{}, "configured": true})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	res := c.RecentlyAdded(ctx)
	type entry struct {
		at time.Time
		v  any
	}
	all := []entry{}
	for _, it := range res.Items {
		it.PosterURL = signedPoster(it.PosterURL)
		all = append(all, entry{it.AddedAt, it})
	}
	for _, m := range s.arrRecent(true) {
		at, _ := m["added_at"].(time.Time)
		all = append(all, entry{at, m})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at.After(all[j].at) })
	if len(all) > 6 {
		all = all[:6]
	}
	items := make([]any, 0, len(all))
	for _, e := range all {
		items = append(items, e.v)
	}
	writeJSON(w, map[string]any{"items": items, "errors": res.Errors, "configured": true})
}

func (s *Server) v1Poster(w http.ResponseWriter, r *http.Request) {
	path, sig := r.URL.Query().Get("path"), r.URL.Query().Get("signature")
	if arrName := r.URL.Query().Get("arr"); arrName != "" {
		if !coverPath.MatchString(path) || !hmac.Equal([]byte(sig), []byte(coverSignature(arrName, path))) {
			http.NotFound(w, r)
			return
		}
		for _, c := range s.Cfg.Get().Categories {
			if c.Name == arrName && c.ArrURL != "" && (c.Kind == "sonarr" || c.Kind == "radarr" || c.Kind == "lidarr") {
				s.serveImage(w, r, strings.TrimRight(c.ArrURL, "/")+path, string(c.ArrAPIKey))
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	if !validPosterPath(path) || !hmac.Equal([]byte(sig), []byte(posterSignature(path))) {
		http.NotFound(w, r)
		return
	}
	base := strings.TrimRight(s.Cfg.Get().Playback.TracearrURL, "/")
	if base == "" {
		http.NotFound(w, r)
		return
	}
	s.serveImage(w, r, base+path, "")
}

func (s *Server) serveImage(w http.ResponseWriter, r *http.Request, target, apiKey string) {
	req, err := http.NewRequestWithContext(r.Context(), "GET", target, nil)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if apiKey != "" {
		req.Header.Set("X-Api-Key", apiKey)
	}
	// Tracearr's image proxy is public; never send the API key to an image URL.
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		if resp != nil {
			resp.Body.Close()
		}
		http.NotFound(w, r)
		return
	}
	defer resp.Body.Close()
	const maxImage = 5 << 20
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImage+1))
	if err != nil || len(body) > maxImage {
		http.NotFound(w, r)
		return
	}
	ct := http.DetectContentType(body)
	switch ct {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
	default:
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=60")
	w.Write(body)
}

// playbackTest checks Tracearr and, if given, Emby. Blank keys mean the stored ones.
func (s *Server) playbackTest(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TracearrURL, TracearrKey, EmbyURL, EmbyKey string
	}
	if !readJSON(w, r, &in) {
		return
	}
	cur := s.Cfg.Get().Playback
	if in.TracearrKey == "" {
		in.TracearrKey = string(cur.TracearrKey)
	}
	if in.EmbyKey == "" {
		in.EmbyKey = string(cur.EmbyKey)
	}
	okURL := func(u string) bool { return strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") }
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	out := map[string]string{}
	if u := strings.TrimSpace(in.TracearrURL); u != "" {
		if !okURL(u) {
			jsonErr(w, http.StatusBadRequest, "the Tracearr address must start with http:// or https://")
			return
		}
		if err := tracearr.New(u, in.TracearrKey).Check(ctx); err != nil {
			jsonErr(w, http.StatusBadGateway, err.Error())
			return
		}
		out["tracearr"] = "ok"
	}
	if u := strings.TrimSpace(in.EmbyURL); u != "" {
		if !okURL(u) {
			jsonErr(w, http.StatusBadRequest, "the Emby address must start with http:// or https://")
			return
		}
		name, err := playback.Emby{URL: u, Key: in.EmbyKey}.Check(ctx)
		if err != nil {
			jsonErr(w, http.StatusBadGateway, "Emby: "+err.Error())
			return
		}
		out["emby"] = name
	}
	writeJSON(w, out)
}
