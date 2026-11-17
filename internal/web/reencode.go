package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/media"
	"github.com/z354prance/overmynd/internal/pipeline"
	"github.com/z354prance/overmynd/internal/store"
)

type reencodeFile struct {
	Path    string    `json:"path"`
	Rel     string    `json:"rel"`
	Size    int64     `json:"size"`
	Changed time.Time `json:"changed"`
	Codec   string    `json:"codec"`
	Error   string    `json:"error,omitempty"`
	Queued  bool      `json:"queued"`
}

// reencodeScan lists library videos that arrived or changed since a date and are not AV1.
func (s *Server) reencodeScan(w http.ResponseWriter, r *http.Request) {
	roots := s.Engine.LibraryRoots
	if len(roots) == 0 {
		jsonErr(w, http.StatusBadRequest, "no media library is set (UNDERMYND_STORAGE_ROOT)")
		return
	}
	since, err := time.Parse(time.RFC3339, r.URL.Query().Get("since"))
	if err != nil {
		jsonErr(w, http.StatusBadRequest, "since must be a date and time")
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(5 * time.Minute))
	found := media.Scan(roots, since)
	out := make([]reencodeFile, 0, len(found))
	for _, f := range found {
		rf := reencodeFile{Path: f.Path, Size: f.Size, Changed: f.Changed, Codec: f.Codec, Error: f.Err}
		rf.Rel = f.Path
		for _, root := range roots {
			if rel, err := filepath.Rel(filepath.Dir(root), f.Path); err == nil && !strings.HasPrefix(rel, "..") {
				rf.Rel = rel
				break
			}
		}
		j, readErr := s.Store.Get(r.Context(), pipeline.ReencodeID(f.Path))
		if readErr != nil && !errors.Is(readErr, store.ErrNotFound) {
			jobStorageError(w)
			return
		}
		if readErr == nil && j.State != job.Cleaned && j.State != job.Failed {
			rf.Queued = true
		}
		out = append(out, rf)
	}
	writeJSON(w, map[string]any{"since": since, "files": out})
}

// reencodeQueue queues the chosen files. Each is checked against the media roots again.
func (s *Server) reencodeQueue(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paths []string `json:"paths"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<20)).Decode(&in); err != nil || len(in.Paths) == 0 {
		jsonErr(w, http.StatusBadRequest, "choose at least one file")
		return
	}
	if len(in.Paths) > 1000 {
		jsonErr(w, http.StatusBadRequest, "queue at most 1000 files at a time")
		return
	}
	queued, skipped := 0, []string{}
	for _, p := range in.Paths {
		if _, err := s.Engine.SubmitReencodeContext(r.Context(), p); err != nil {
			skipped = append(skipped, filepath.Base(p)+": "+err.Error())
			continue
		}
		queued++
	}
	s.Log.Printf("admin: queued %d file(s) for re-encoding (%d skipped)", queued, len(skipped))
	writeJSON(w, map[string]any{"queued": queued, "skipped": skipped})
}

// logLines serves the newest container log lines, optionally narrowed to lines that
// contain every word of q (a job id, "tdarr", "failed").
func (s *Server) logLines(w http.ResponseWriter, r *http.Request) {
	n := 300
	if v, err := strconv.Atoi(r.URL.Query().Get("n")); err == nil && v > 0 && v <= 2000 {
		n = v
	}
	lines := []string{}
	if s.Logs != nil {
		lines = s.Logs.Lines(r.URL.Query().Get("q"), n)
	}
	writeJSON(w, map[string]any{"lines": lines})
}
