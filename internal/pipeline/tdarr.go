package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/tdarr"
)

var videoExt = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".ts": true, ".m2ts": true, ".wmv": true, ".webm": true,
}

func isVideo(p string) bool { return videoExt[strings.ToLower(filepath.Ext(p))] }

func stem(p string) string { return strings.TrimSuffix(p, filepath.Ext(p)) }

func (e *Engine) tdarrIn(id string) string  { return filepath.Join(e.Cfg.Get().Tdarr.InDir, id) }
func (e *Engine) tdarrOut(id string) string { return filepath.Join(e.Cfg.Get().Tdarr.OutDir, id) }

// sendToTdarr moves the staged videos into the Tdarr input folder. Sidecar files
// (subtitles) stay in staging. It returns false when there is nothing for Tdarr to do.
func (e *Engine) sendToTdarr(ctx context.Context, j job.Job) bool {
	var videos []string
	for _, f := range j.Files {
		if isVideo(f) {
			videos = append(videos, f)
		}
	}
	if len(videos) == 0 {
		return false
	}
	inFlight, err := e.tdarrInFlight(ctx)
	if err != nil {
		e.Log.Printf("Tdarr hand-off stopped: repository read failed: %v", err)
		return true
	}
	if max := e.Cfg.Get().Tdarr.MaxInFlight; max > 0 && inFlight >= max {
		if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Reason = fmt.Sprintf("waiting for a free Tdarr slot (limit %d)", max)
			c.NextRun = e.Now().Add(30 * time.Second)
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return true
		}
		return true
	}
	for _, f := range videos {
		if err := moveFile(filepath.Join(j.StagingDir, f), filepath.Join(e.tdarrIn(j.ID), f)); err != nil {
			e.retry(j, err, 6, "move to Tdarr input")
			return true
		}
	}
	if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
		c.TranscodeSig = ""
		c.LastProgress = e.Now()
		c.Move(job.QueuedTranscode, fmt.Sprintf("%d video(s) waiting in the Tdarr input folder", len(videos)), e.Now())
		return nil
	}); writeErr != nil {
		e.Log.Printf("job state update failed: %v", writeErr)
		return true
	}
	e.Log.Printf("job %s sent to Tdarr", short(j.ID))
	if cli, lib := e.tdarrClient(), e.Cfg.Get().Tdarr.LibraryID; cli != nil && lib != "" {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if err := cli.Scan(ctx, lib, e.Cfg.Get().Tdarr.InDir); err != nil {
			e.Log.Printf("job %s tdarr scan request: %v (the library's folder watcher will still find it)", short(j.ID), err)
		}
	}
	return true
}

// findOutput looks for Tdarr's result for one input video: same folder, same name, any video extension.
func (e *Engine) findOutput(id, rel string) (string, os.FileInfo) {
	dir := filepath.Join(e.tdarrOut(id), filepath.Dir(rel))
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", nil
	}
	want := filepath.Base(stem(rel))
	for _, en := range ents {
		if en.IsDir() || !isVideo(en.Name()) || strings.HasPrefix(en.Name(), ".") {
			continue
		}
		if stem(en.Name()) == want {
			if fi, err := en.Info(); err == nil {
				return filepath.Join(dir, en.Name()), fi
			}
		}
	}
	return "", nil
}

// tdarrClient returns the API client, or nil when no Tdarr address is set. Without
// it the folder hand-off still works; the API only adds progress and early failure.
func (e *Engine) tdarrClient() *tdarr.Client {
	t := e.Cfg.Get().Tdarr
	if t.URL == "" {
		return nil
	}
	return tdarr.New(t.URL, string(t.APIKey))
}

// tdarrInFlight counts the jobs currently with Tdarr.
func (e *Engine) tdarrInFlight(ctx context.Context) (int, error) {
	n := 0
	jobs, err := e.Store.List(ctx)
	if err != nil {
		return 0, err
	}
	for _, j := range jobs {
		switch j.State {
		case job.QueuedTranscode, job.Transcoding, job.Verifying:
			n++
		}
	}
	return n, nil
}

// tdarrWorkerOn returns the Tdarr worker that currently holds one of this job's files, or nil.
// It matches on the job's own folder name, so a node that mounts the share at a different
// path still finds its worker.
func (e *Engine) tdarrWorkerOn(ctx context.Context, j job.Job) *tdarr.Worker {
	cli := e.tdarrClient()
	if cli == nil {
		return nil
	}
	ws, err := cli.ActiveWorkers(ctx)
	if err != nil {
		return nil
	}
	prefix := filepath.ToSlash(e.tdarrIn(j.ID)) + "/"
	inJob := "/" + j.ID + "/"
	for i := range ws {
		p := filepath.ToSlash(ws[i].File)
		if strings.HasPrefix(p, prefix) || strings.Contains(p, inJob) {
			return &ws[i]
		}
	}
	return nil
}

// transcodeFailed applies the category's failure policy. It never acts while a Tdarr
// worker is still encoding the file: handing back the original (or failing the grab, which
// clears the Tdarr folders) would pull the file out from under the encode, and the arr
// would import an untranscoded copy while Tdarr carries on.
func (e *Engine) transcodeFailed(j job.Job, why string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	w := e.tdarrWorkerOn(ctx, j)
	cancel()
	if w != nil && !e.Now().After(j.StateChanged.Add(time.Duration(e.Cfg.Get().Tdarr.TimeoutMinutes)*time.Minute)) {
		e.Log.Printf("job %s: %s, but Tdarr worker %s on %s is still on the file (%.0f%%); not handing it back", short(j.ID), why, w.Worker, w.Node, w.Percentage)
		if _, writeErr := e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
			c.Reason = "Tdarr is still encoding"
			c.NextRun = e.Now().Add(30 * time.Second)
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return
		}
		return
	}
	if j.Source == "reencode" {
		// The library file is untouched; there is nothing to hand back.
		e.fail(j, why)
		return
	}
	cat, _ := e.Cfg.Get().Category(j.Category)
	if cat.OnTranscodeFail == "passthrough" {
		if err := e.passthrough(j); err == nil {
			e.Log.Printf("job %s: %s; handing back the original", short(j.ID), why)
			return
		}
	}
	e.fail(j, why)
}

// passthrough puts the untranscoded originals back into staging and hands them back.
func (e *Engine) passthrough(j job.Job) error {
	if j.Source == "reencode" {
		// Tdarr says the file needs no change: leave the library file alone and finish.
		if err := e.removeTdarrDirs(j); err != nil {
			return err
		}
		if err := e.removeTree(e.Cfg.Env.StagingDir(j.ID), e.Cfg.Env.Work()); err != nil {
			return err
		}
		if _, writeErr := e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
			c.Move(job.Cleaned, "Tdarr says no change is needed; the library file was left alone", e.Now())
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return writeErr
		}
		return nil
	}
	for _, f := range j.Files {
		if !isVideo(f) {
			continue
		}
		if err := moveFile(filepath.Join(e.tdarrIn(j.ID), f), filepath.Join(j.StagingDir, f)); err != nil {
			return err
		}
	}
	if err := e.removeTdarrDirs(j); err != nil {
		return err
	}
	if _, writeErr := e.Store.Update(context.Background(), j.ID, func(c *job.Job) error { c.TranscodeSig = ""; c.Reason = "transcode skipped, original kept"; return nil }); writeErr != nil {
		e.Log.Printf("job state update failed: %v", writeErr)
		return writeErr
	}
	e.handBack(j)
	return nil
}

// doTranscode watches Tdarr (by API when configured, always by folder) until every
// video's output exists and has stopped growing.
func (e *Engine) doTranscode(ctx context.Context, j job.Job) {
	now := e.Now()
	s := e.Cfg.Get()
	// Tdarr is asked often: it is a local call, and the wait between Tdarr starting
	// and Overmynd noticing is time the person sits through.
	next := now.Add(5 * time.Second)
	cli := e.tdarrClient()

	found, total := 0, 0
	var sig strings.Builder
	var rec *tdarr.FileRecord
	var worker *tdarr.Worker
	for _, f := range j.Files {
		if !isVideo(f) {
			continue
		}
		total++
		if _, fi := e.findOutput(j.ID, f); fi != nil {
			found++
			fmt.Fprintf(&sig, "%d,", fi.Size())
		}
	}
	if cli != nil {
		if ws, err := cli.ActiveWorkers(ctx); err == nil {
			// Match on the job's own folder name, so a node that mounts the share
			// at a different path still finds its worker.
			prefix := filepath.ToSlash(e.tdarrIn(j.ID)) + "/"
			inJob := "/" + j.ID + "/"
			for i := range ws {
				p := filepath.ToSlash(ws[i].File)
				if strings.HasPrefix(p, prefix) || strings.Contains(p, inJob) {
					worker = &ws[i]
					break
				}
			}
			if worker == nil && len(ws) > 0 && j.State == job.QueuedTranscode && now.Sub(j.StateChanged) > time.Minute {
				e.Log.Printf("job %s: %d Tdarr worker(s) busy, none on this job's folder (first: %s)", short(j.ID), len(ws), ws[0].File)
			}
		} else {
			e.Log.Printf("job %s tdarr workers: %v", short(j.ID), err)
		}
		for _, f := range j.Files {
			if !isVideo(f) {
				continue
			}
			r, err := cli.Record(ctx, filepath.Join(e.tdarrIn(j.ID), f))
			if err != nil {
				e.Log.Printf("job %s tdarr record: %v", short(j.ID), err)
				break
			}
			if r != nil {
				rec = r
				if tdarr.Failed(r) {
					break
				}
			}
		}
	}

	// A worker holding the file is progress; Tdarr's own status words can say it gave up.
	if rec != nil && tdarr.Failed(rec) && found < total {
		e.transcodeFailed(j, "Tdarr reported: "+strings.TrimSpace(rec.Transcode+" "+rec.HealthCheck))
		return
	}
	if worker != nil {
		// Progress is the percentage moving or the encoder reporting frames. A hardware AV1
		// encode can sit at one percentage for a long while, so the idle limit has a floor.
		moved := worker.Percentage/100 != j.TranscodeProgress || worker.FPS > 0
		if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.TranscodeProgress, c.TranscodeNode, c.TranscodeWorker = worker.Percentage/100, worker.Node, worker.Worker
			if moved {
				c.LastProgress = now
			}
			if c.State == job.QueuedTranscode {
				c.Move(job.Transcoding, fmt.Sprintf("transcoding on %s", worker.Node), now)
			}
			c.NextRun = next
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return
		}
		stall := s.Tdarr.StallMinutes
		if stall < 60 {
			stall = 60
		}
		if !moved && now.Sub(j.LastProgress) > time.Duration(stall)*time.Minute {
			e.transcodeFailed(j, fmt.Sprintf("Tdarr made no progress for %d minutes", stall))
		}
		return
	}

	timeout := time.Duration(s.Tdarr.TimeoutMinutes) * time.Minute
	if found == 0 {
		// Tdarr decided nothing needed doing and copied nothing: keep the original.
		if rec != nil && tdarr.NotRequired(rec) && now.Sub(j.StateChanged) > 5*time.Minute {
			if err := e.passthrough(j); err == nil {
				return
			}
		}
		if now.Sub(j.StateChanged) > timeout {
			e.transcodeFailed(j, fmt.Sprintf("Tdarr produced no output within %d minutes", s.Tdarr.TimeoutMinutes))
			return
		}
		if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.NextRun = next; return nil }); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return
		}
		return
	}
	if j.State == job.QueuedTranscode {
		if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Move(job.Transcoding, "Tdarr is producing output", now)
			c.TranscodeSig, c.NextRun = sig.String(), next
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return
		}
		return
	}
	if found == total && sig.String() == j.TranscodeSig {
		if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.TranscodeProgress = 1
			c.Move(job.Verifying, "Tdarr output complete", now)
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return
		}
		return
	}
	if now.Sub(j.StateChanged) > timeout {
		e.transcodeFailed(j, fmt.Sprintf("Tdarr did not finish within %d minutes", s.Tdarr.TimeoutMinutes))
		return
	}
	if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.TranscodeSig, c.NextRun = sig.String(), next; return nil }); writeErr != nil {
		e.Log.Printf("job state update failed: %v", writeErr)
		return
	}
}

// validContainer checks the file starts like the container its extension claims.
func validContainer(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	h := make([]byte, 12)
	if n, _ := f.Read(h); n < 12 {
		return false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mkv", ".webm":
		return h[0] == 0x1A && h[1] == 0x45 && h[2] == 0xDF && h[3] == 0xA3
	case ".mp4", ".m4v", ".mov":
		return string(h[4:8]) == "ftyp" || string(h[4:8]) == "moov" || string(h[4:8]) == "mdat" || string(h[4:8]) == "free"
	}
	return true
}

// doVerify checks each output, swaps it into staging in place of the original, clears the Tdarr folders and hands back.
func (e *Engine) doVerify(ctx context.Context, j job.Job) {
	type swap struct{ rel, out string }
	var swaps []swap
	for _, f := range j.Files {
		if !isVideo(f) {
			continue
		}
		out, fi := e.findOutput(j.ID, f)
		if fi == nil {
			e.fail(j, "Tdarr output for "+f+" vanished")
			return
		}
		in, err := os.Stat(filepath.Join(e.tdarrIn(j.ID), f))
		if fi.Size() == 0 || (err == nil && in.Size() > 0 && fi.Size()*20 < in.Size()) {
			e.fail(j, "Tdarr output for "+f+" failed the size check")
			return
		}
		if !validContainer(out) {
			e.fail(j, "Tdarr output for "+f+" is not a valid media container")
			return
		}
		swaps = append(swaps, swap{f, out})
	}
	newFiles := make([]string, 0, len(j.Files))
	repl := map[string]string{}
	for _, sw := range swaps {
		dstRel := filepath.Join(filepath.Dir(sw.rel), filepath.Base(sw.out))
		if err := moveFile(sw.out, filepath.Join(j.StagingDir, dstRel)); err != nil {
			e.retry(j, err, 6, "move Tdarr output to staging")
			return
		}
		repl[sw.rel] = dstRel
	}
	for _, f := range j.Files {
		if r, ok := repl[f]; ok {
			newFiles = append(newFiles, r)
		} else {
			newFiles = append(newFiles, f)
		}
	}
	if err := e.removeTdarrDirs(j); err != nil {
		e.retry(j, err, 10, "clear Tdarr folders")
		return
	}
	if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.Files, c.TranscodeSig = newFiles, ""; return nil }); writeErr != nil {
		e.Log.Printf("job state update failed: %v", writeErr)
		return
	}
	j.Files = newFiles
	e.handBack(j)
}

func (e *Engine) removeTdarrDirs(j job.Job) error {
	s := e.Cfg.Get().Tdarr
	if err := e.removeTree(filepath.Join(s.InDir, j.ID), s.InDir); err != nil {
		return err
	}
	return e.removeTree(filepath.Join(s.OutDir, j.ID), s.OutDir)
}
