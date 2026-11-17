package web

import (
	"net/http"
	"strings"

	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/storage"
)

// The public dashboard is Overmynd's page (static/dash), unchanged except for the
// name and logo. It reads the same /api/v1 routes Overmynd served; Overmynd answers
// them from its own job store and the arrs, so the page needs no other change.

func (s *Server) dashboardRoutes(mux *http.ServeMux, files http.Handler) {
	mux.Handle("GET /dash/", files)
	mux.HandleFunc("GET /api/v1/activity", s.v1Activity)
	// Same data under a name common ad-block lists do not match (some block /api/v1/activity).
	mux.HandleFunc("GET /api/v1/media-progress", s.v1Activity)
	mux.HandleFunc("GET /api/v1/downloads", func(w http.ResponseWriter, r *http.Request) {
		d, _, _ := s.dashboardRecords()
		writeJSON(w, map[string]any{"downloads": d, "errors": []string{}})
	})
	mux.HandleFunc("GET /api/v1/processing", func(w http.ResponseWriter, r *http.Request) {
		_, p, _ := s.dashboardRecords()
		writeJSON(w, map[string]any{"jobs": p, "errors": []string{}})
	})
	mux.HandleFunc("GET /api/v1/recently-added", s.v1Recent)
	mux.HandleFunc("GET /api/v1/storage", s.v1Storage)
	mux.HandleFunc("GET /api/v1/playback", s.v1Playback)
	mux.HandleFunc("GET /api/v1/playback/poster", s.v1Poster)
	mux.HandleFunc("GET /api/v1/missing", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"items": []any{}, "errors": []string{}})
	})
	mux.HandleFunc("GET /api/v1/public/services", func(w http.ResponseWriter, r *http.Request) {
		out := []map[string]any{}
		for _, t := range []string{"torboxarr", "qbittorrent", "nzbget", "tdarr", "undermynd"} {
			out = append(out, map[string]any{"id": serviceIDs[t], "name": serviceNames[t], "type": t, "enabled": true})
		}
		writeJSON(w, map[string]any{"services": out})
	})
	mux.HandleFunc("GET /api/v1/auth/status", func(w http.ResponseWriter, r *http.Request) {
		// "authenticated" stays false: the public page has no admin screens here. "admin" only
		// switches on the approve/deny buttons for the Overmynd admin session.
		writeJSON(w, map[string]any{"authenticated": false, "setup_required": false, "admin": s.signedIn(r)})
	})
	mux.HandleFunc("GET /api/v1/media-request/status", s.requestStatus)
	mux.HandleFunc("GET /api/v1/media-request/search", s.requestSearch)
	mux.HandleFunc("GET /api/v1/media-request/discover", s.requestDiscover)
	mux.HandleFunc("POST /api/v1/media-request", s.requestCreate)
	mux.HandleFunc("GET /api/v1/requests", s.requestsPending)
	mux.HandleFunc("POST /api/v1/requests/{serviceID}/{requestID}/{action}", s.guard(s.admin(s.requestsDecide)))
	mux.HandleFunc("GET /api/v1/donations", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.Cfg.Get().Donations)
	})
	s.accessRoutes(mux)
}

// Each place a job can be is a "service" to the page, so cards say where it is.
var serviceIDs = map[string]int64{"torboxarr": 1, "qbittorrent": 2, "nzbget": 3, "tdarr": 4, "undermynd": 5}
var serviceNames = map[string]string{"torboxarr": "TorBox", "qbittorrent": "qBittorrent", "nzbget": "NZBGet", "tdarr": "Tdarr", "undermynd": "Overmynd"}

type v1Download struct {
	ID              string   `json:"id"`
	Source          string   `json:"source"`
	SourceServiceID int64    `json:"source_service_id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	Progress        *float64 `json:"progress,omitempty"`
	Size            int64    `json:"size,omitempty"`
	SizeLeft        int64    `json:"size_left"`
}

type v1Process struct {
	ID              string  `json:"id"`
	Source          string  `json:"source"`
	SourceServiceID int64   `json:"source_service_id"`
	Title           string  `json:"title"`
	State           string  `json:"state"`
	Stage           string  `json:"stage,omitempty"`
	Progress        float64 `json:"progress,omitempty"`
	NodeName        string  `json:"node_name,omitempty"`
}

type v1Ref struct {
	Source          string `json:"source"`
	SourceServiceID int64  `json:"source_service_id"`
	RecordType      string `json:"record_type"`
	RecordID        string `json:"record_id"`
}

type v1Lifecycle struct {
	ID         string   `json:"id"`
	Kind       string   `json:"kind,omitempty"`
	Title      string   `json:"title"`
	Stage      string   `json:"stage"`
	Problems   []string `json:"problems,omitempty"`
	FailedAt   string   `json:"failed_at,omitempty"`
	Backend    string   `json:"backend,omitempty"` // torbox, qbit or nzbget: torbox adds a Transfer step
	Progress   float64  `json:"progress,omitempty"`
	References []v1Ref  `json:"references"`
}

// dashboardRecords turns every unfinished job into the download and processing
// records the page correlates, plus one lifecycle per job.
func (s *Server) dashboardRecords() ([]v1Download, []v1Process, []v1Lifecycle) {
	downloads, procs, lifes := []v1Download{}, []v1Process{}, []v1Lifecycle{}
	for _, j := range s.Store.List() {
		if j.State == job.Cleaned || j.Source == "reencode" {
			continue
		}
		kind := map[string]string{"sonarr": "episode", "radarr": "movie", "lidarr": "album"}[j.Source]
		if kind == "" {
			if c, ok := s.Cfg.Get().Category(j.Category); ok {
				kind = map[string]string{"sonarr": "episode", "radarr": "movie", "lidarr": "album"}[c.Kind]
			}
		}
		l := v1Lifecycle{ID: "job:" + j.ID, Kind: kind, Title: j.Name, Backend: j.Backend, References: []v1Ref{}}
		src := "qbittorrent"
		switch {
		case j.Backend == "torbox":
			src = "torboxarr"
		case j.Kind == "usenet":
			src = "nzbget"
		}
		addDownload := func(status string, size, left int64, pct *float64) {
			downloads = append(downloads, v1Download{ID: j.ID, Source: src, SourceServiceID: serviceIDs[src], Title: j.Name, Status: status, Progress: pct, Size: size, SizeLeft: left})
			l.References = append(l.References, v1Ref{Source: src, SourceServiceID: serviceIDs[src], RecordType: "download", RecordID: j.ID})
		}
		addProcess := func(p v1Process) {
			p.ID, p.Title, p.SourceServiceID = j.ID, j.Name, serviceIDs[p.Source]
			procs = append(procs, p)
			l.References = append(l.References, v1Ref{Source: p.Source, SourceServiceID: serviceIDs[p.Source], RecordType: "processing", RecordID: j.ID})
		}
		pct := func(f float64) *float64 { v := f * 100; return &v }
		switch j.State {
		case job.Received, job.Submitted:
			l.Stage = "downloading"
			addDownload("queued", j.Size, j.Size, nil)
		case job.Downloading:
			l.Stage = "downloading"
			addDownload("downloading", j.Size, j.Size-int64(j.Progress*float64(j.Size)), pct(j.Progress))
		case job.Pulling, job.Downloaded:
			if j.State == job.Downloaded && strings.Contains(j.Reason, "Tdarr slot") {
				// Copied and verified; it is only queued for a free transcode slot.
				l.Stage = "processing"
				addProcess(v1Process{Source: "undermynd", State: "queued", Stage: "transcode"})
				break
			}
			l.Stage = "downloading"
			total, done := j.PullTotal, j.PullDone
			if j.State == job.Downloaded || total == 0 {
				total, done = max(j.Size, total), max(j.Size, total)
			}
			f := 1.0
			if total > 0 {
				f = float64(done) / float64(total)
			}
			addDownload("pulling", total, total-done, pct(f))
		case job.QueuedTranscode:
			l.Stage = "processing"
			addProcess(v1Process{Source: "undermynd", State: "queued", Stage: "transcode"})
		case job.Transcoding:
			l.Stage = "processing"
			addProcess(v1Process{Source: "tdarr", State: "processing", Stage: "transcode", Progress: j.TranscodeProgress * 100, NodeName: j.TranscodeNode})
		case job.Verifying:
			l.Stage = "processing"
			addProcess(v1Process{Source: "undermynd", State: "processing", Stage: "verify", Progress: 100})
		case job.Failed:
			l.Stage, l.Problems, l.FailedAt = "importing", []string{"failed"}, job.FailedStage(j)
			l.Progress = j.Progress * 100
		default: // ready, imported: handed back, waiting for the arr
			l.Stage = "importing"
		}
		lifes = append(lifes, l)
	}
	return downloads, procs, lifes
}

func (s *Server) v1Activity(w http.ResponseWriter, r *http.Request) {
	d, p, l := s.dashboardRecords()
	seasons := []map[string]any{}
	if s.Progress != nil {
		for _, n := range s.Progress.Snapshot().Seasons {
			seasons = append(seasons, map[string]any{
				"show_id": n.ShowID, "show_title": n.Show, "id": n.ShowID + ":" + itoa(n.Season),
				"title": n.Show + " — Season " + itoa(n.Season), "season_number": n.Season, "imported": n.Imported, "total": n.Total,
			})
		}
	}
	writeJSON(w, map[string]any{
		"lifecycles": l, "seasons": seasons, "errors": []string{},
		"download_snapshot":   map[string]any{"downloads": d, "errors": []string{}},
		"processing_snapshot": map[string]any{"jobs": p, "errors": []string{}},
	})
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// v1Storage serves the cached library sizes and the free space of the media
// filesystem, exactly as Overmynd's storage monitor did (UNDERMYND_STORAGE_ROOT).
func (s *Server) v1Storage(w http.ResponseWriter, r *http.Request) {
	snap := storage.Snapshot{Enabled: false, Categories: []storage.Category{}}
	if s.Storage != nil {
		snap = s.Storage.Snapshot()
	}
	writeJSON(w, snap)
}
