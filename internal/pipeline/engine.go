// Package pipeline runs every job from hand-off to cleanup.
package pipeline

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/z354prance/overmynd/internal/arr"
	"github.com/z354prance/overmynd/internal/backend"
	"github.com/z354prance/overmynd/internal/bencode"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/store"
)

type ImportChecker interface {
	Imported(ctx context.Context, downloadID string) (bool, error)
}

// DownloadFailer is implemented by an arr client that can be told a download failed.
type DownloadFailer interface {
	FailDownload(ctx context.Context, downloadID string) (bool, error)
}

type Engine struct {
	Store store.Repository
	Cfg   *config.Config
	Log   *log.Logger
	Now   func() time.Time

	// Override replaces every backend (tests).
	Override backend.Backend
	// ArrFor replaces the arr client lookup (tests).
	ArrFor func(config.Category) ImportChecker

	submitMu sync.Mutex
	bmu      sync.Mutex
	qkey     string
	ngkey    string
	ngcache  *backend.NZBGet
	qcache   backend.Backend
	tkey     string
	tcache   *backend.TorBox
	ttcache  *backend.TorBox
	lim      *backend.Limiter
	nmu      sync.Mutex
	notes    map[string]string

	// LibraryRoots are the media folders re-encode jobs may read and replace files in.
	LibraryRoots []string

	pulling sync.Map // job ID -> running
	wg      sync.WaitGroup
	sem     chan struct{}
}

func New(s store.LegacyRepository, c *config.Config, l *log.Logger) *Engine {
	return NewRepository(store.FromLegacy(s), c, l)
}

// NewRepository accepts error-aware storage without selecting SQLite at startup.
func NewRepository(s store.Repository, c *config.Config, l *log.Logger) *Engine {
	if l == nil {
		l = log.New(io.Discard, "", 0)
	}
	return &Engine{Store: s, Cfg: c, Log: l, Now: time.Now, sem: make(chan struct{}, max(1, c.Get().TorBox.PullersAtOnce))}
}

// Wait blocks until background pulls have finished (tests).
func (e *Engine) Wait() { e.wg.Wait() }

func (e *Engine) qbit() backend.Backend {
	s := e.Cfg.Get()
	key := s.QBit.URL + "|" + s.QBit.Username + "|" + string(s.QBit.Password) + "|" + s.QBit.SaveDir
	e.bmu.Lock()
	defer e.bmu.Unlock()
	if e.qcache == nil || e.qkey != key {
		e.qcache = backend.NewQBit(s.QBit.URL, s.QBit.Username, string(s.QBit.Password), s.QBit.SaveDir)
		e.qkey = key
	}
	return e.qcache
}

func (e *Engine) torbox() *backend.TorBox {
	s := e.Cfg.Get().TorBox
	if s.Token == "" {
		return nil
	}
	key := s.BaseURL + "|" + string(s.Token) + fmt.Sprint(s.CreatePerHour, s.LinkGapSeconds)
	e.bmu.Lock()
	defer e.bmu.Unlock()
	if e.tcache == nil || e.tkey != key {
		e.lim = backend.NewLimiter(filepath.Join(e.Cfg.Env.ConfigDir, "torbox-limiter.json"), s.CreatePerHour,
			time.Duration(s.LinkGapSeconds)*time.Second)
		e.lim.Now = func() time.Time { return e.Now() }
		e.tcache = backend.NewTorBox(s.BaseURL, string(s.Token), e.lim)
		e.tcache.Now = func() time.Time { return e.Now() }
		e.tcache.Logf = e.Log.Printf
		e.ttcache = backend.NewTorBoxTorrents(s.BaseURL, string(s.Token), e.lim)
		e.ttcache.Now = e.tcache.Now
		e.ttcache.Logf = e.Log.Printf
		e.tkey = key
	}
	return e.tcache
}

func (e *Engine) nzbget() *backend.NZBGet {
	n := e.Cfg.Get().NZBGet
	if n.URL == "" {
		return nil
	}
	key := n.URL + "|" + n.Username + "|" + string(n.Password)
	e.bmu.Lock()
	defer e.bmu.Unlock()
	if e.ngcache == nil || e.ngkey != key {
		e.ngcache, e.ngkey = backend.NewNZBGet(n.URL, n.Username, string(n.Password)), key
	}
	return e.ngcache
}

// pick chooses the backend for a new job.
func (e *Engine) pick(j job.Job) (backend.Backend, error) {
	if e.Override != nil {
		return e.Override, nil
	}
	if j.Kind == "usenet" {
		if tb := e.torbox(); tb != nil {
			return tb, nil
		}
		if ng := e.nzbget(); ng != nil {
			return ng, nil
		}
		return nil, backend.Permanent{Msg: "no usenet backend is configured: set a TorBox token or an NZBGet address"}
	}
	if tb := e.torboxTorrents(); tb != nil {
		return tb, nil
	}
	return e.qbit(), nil
}

func (e *Engine) torboxTorrents() *backend.TorBox {
	if e.torbox() == nil {
		return nil
	}
	e.bmu.Lock()
	defer e.bmu.Unlock()
	return e.ttcache
}

// byName returns the backend a job was already submitted to.
func (e *Engine) byName(name string, j job.Job) backend.Backend {
	if e.Override != nil {
		return e.Override
	}
	if name == "torbox" {
		if tb := e.torbox(); tb != nil {
			if j.Kind != "usenet" {
				return e.torboxTorrents()
			}
			return tb
		}
	}
	if name == "nzbget" {
		if ng := e.nzbget(); ng != nil {
			return ng
		}
	}
	return e.qbit()
}

func (e *Engine) backend(j job.Job) backend.Backend { return e.byName(j.Backend, j) }

func (e *Engine) arrFor(c config.Category) ImportChecker {
	if e.ArrFor != nil {
		return e.ArrFor(c)
	}
	if c.ArrURL == "" || c.Kind == "" || c.Kind == "rss" {
		return nil
	}
	return arr.New(c.Kind, c.ArrURL, string(c.ArrAPIKey))
}

// ---------------------------------------------------------------- intake

type SubmitRequest struct {
	Source   string // overrides the arr kind, e.g. "rss"
	Category string
	Magnet   string
	Torrent  []byte
}

// Submit creates (or returns) the job for a grab. The job ID is the infohash,
// so it matches the download ID the arr records.
func (e *Engine) Submit(req SubmitRequest) (job.Job, error) {
	var id, name string
	var size int64
	var err error
	switch {
	case len(req.Torrent) > 0:
		id, name, size, err = bencode.Torrent(req.Torrent)
	case req.Magnet != "":
		id, name, err = bencode.Magnet(req.Magnet)
	default:
		err = errors.New("no torrent or magnet supplied")
	}
	if err != nil {
		return job.Job{}, err
	}
	if name == "" {
		name = id
	}
	e.submitMu.Lock()
	defer e.submitMu.Unlock()
	cur, readErr := e.Store.Get(context.Background(), id)
	if readErr != nil && !errors.Is(readErr, store.ErrNotFound) {
		return job.Job{}, fmt.Errorf("read job before submission: %w", readErr)
	}
	if readErr == nil && cur.State != job.Cleaned && cur.State != job.Failed {
		return cur, nil // the arr asked again; same job
	}
	now := e.Now()
	j := job.Job{ID: id, Name: name, Category: req.Category, Kind: "torrent", State: job.Received,
		Size: size, Created: now, Updated: now, StateChanged: now, LastProgress: now}
	if c, ok := e.Cfg.Get().Category(req.Category); ok {
		j.Source = c.Kind
	} else {
		j.Source = "unknown"
	}
	if req.Source != "" {
		j.Source = req.Source
	}
	if len(req.Torrent) > 0 {
		dir := filepath.Join(e.Cfg.Env.ConfigDir, "torrents")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return job.Job{}, err
		}
		j.TorrentPath = filepath.Join(dir, id+".torrent")
		if err := os.WriteFile(j.TorrentPath, req.Torrent, 0o600); err != nil {
			return job.Job{}, err
		}
	} else {
		j.Magnet = req.Magnet
	}
	j.History = []job.Transition{{At: now, To: job.Received, Reason: "grab received from " + j.Source}}
	if err := e.Store.Put(context.Background(), j); err != nil {
		return job.Job{}, err
	}
	e.Log.Printf("job %s received (%s, category %s)", short(id), name, req.Category)
	return j, nil
}

type SubmitNZBRequest struct {
	Source   string
	Category string
	Name     string
	NZB      []byte
}

// nzo IDs look like SABnzbd's, and are derived from the NZB so a repeated grab is the same job.
func nzoID(nzb []byte) string {
	h := sha1.Sum(nzb)
	return "SABnzbd_nzo_" + hex.EncodeToString(h[:])[:12]
}

func nzbSize(nzb []byte) int64 {
	var doc struct {
		Files []struct {
			Segments []struct {
				Bytes int64 `xml:"bytes,attr"`
			} `xml:"segments>segment"`
		} `xml:"file"`
	}
	if xml.Unmarshal(nzb, &doc) != nil {
		return 0
	}
	var n int64
	for _, f := range doc.Files {
		for _, s := range f.Segments {
			n += s.Bytes
		}
	}
	return n
}

// SubmitNZB creates (or returns) the job for a usenet grab. The returned ID is
// what the arr records as the download ID.
func (e *Engine) SubmitNZB(req SubmitNZBRequest) (job.Job, error) {
	if len(req.NZB) == 0 || !strings.Contains(string(req.NZB[:min(len(req.NZB), 2048)]), "<nzb") {
		return job.Job{}, errors.New("not an NZB file")
	}
	id := nzoID(req.NZB)
	name := strings.TrimSuffix(strings.TrimSpace(req.Name), ".nzb")
	if name == "" {
		name = id
	}
	e.submitMu.Lock()
	defer e.submitMu.Unlock()
	cur, readErr := e.Store.Get(context.Background(), id)
	if readErr != nil && !errors.Is(readErr, store.ErrNotFound) {
		return job.Job{}, fmt.Errorf("read job before submission: %w", readErr)
	}
	if readErr == nil && cur.State != job.Cleaned && cur.State != job.Failed {
		return cur, nil
	}
	dir := filepath.Join(e.Cfg.Env.ConfigDir, "nzbs")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return job.Job{}, err
	}
	path := filepath.Join(dir, id+".nzb")
	if err := os.WriteFile(path, req.NZB, 0o600); err != nil {
		return job.Job{}, err
	}
	now := e.Now()
	j := job.Job{ID: id, Name: name, Category: req.Category, Kind: "usenet", State: job.Received,
		Size: nzbSize(req.NZB), NZBPath: path, Created: now, Updated: now, StateChanged: now, LastProgress: now}
	if c, ok := e.Cfg.Get().Category(req.Category); ok {
		j.Source = c.Kind
	} else {
		j.Source = "unknown"
	}
	if req.Source != "" {
		j.Source = req.Source
	}
	j.History = []job.Transition{{At: now, To: job.Received, Reason: "grab received from " + j.Source}}
	if err := e.Store.Put(context.Background(), j); err != nil {
		return job.Job{}, err
	}
	e.Log.Printf("job %s received (usenet %s, category %s)", short(id), name, req.Category)
	return j, nil
}

// ArrDelete handles the arr's delete call for an item.
func (e *Engine) ArrDelete(id string) error {
	_, err := e.Store.Update(context.Background(), id, func(j *job.Job) error {
		j.ArrDeleted = true
		switch j.State {
		case job.Ready, job.Imported, job.Cleaned, job.Failed:
			// the loop cleans up (or already has)
		default:
			j.Move(job.Failed, "cancelled by the arr", e.Now())
		}
		return nil
	})
	return err
}

// ------------------------------------------------------------ admin actions

func (e *Engine) Retry(id string) error {
	_, err := e.Store.Update(context.Background(), id, func(j *job.Job) error {
		if j.State != job.Failed {
			return errors.New("only a failed job can be retried")
		}
		j.FailCleaned, j.ArrDeleted, j.ArrNotified, j.Flag = false, false, false, ""
		if j.RemoteID != "" {
			j.PastRemoteIDs = append(j.PastRemoteIDs, j.RemoteID)
		}
		j.RemoteID, j.SavePath, j.StagingDir, j.CompleteDir, j.Files = "", "", "", "", nil
		j.Progress, j.Retries = 0, 0
		j.LastProgress = e.Now()
		j.Move(job.Received, "retried by admin", e.Now())
		return nil
	})
	return err
}

func (e *Engine) Cancel(id string) error {
	_, err := e.Store.Update(context.Background(), id, func(j *job.Job) error {
		if j.State == job.Cleaned {
			return nil
		}
		j.ArrDeleted = true // nothing will be told to the arr; clean up and finish
		if j.State != job.Failed && j.State != job.Imported {
			j.Move(job.Failed, "cancelled by admin", e.Now())
		}
		return nil
	})
	return err
}

// FailJob fails a job on purpose, so the arr sees a failed download and blocklists the release.
func (e *Engine) FailJob(id string) error {
	_, err := e.Store.Update(context.Background(), id, func(j *job.Job) error {
		switch j.State {
		case job.Failed, job.Cleaned:
			return errors.New("the job has already ended")
		}
		j.Move(job.Failed, "failed by admin", e.Now())
		return nil
	})
	return err
}

// CleanupNow runs a finished or failed job's cleanup right now and reports what
// went wrong, so the admin button shows a result instead of silently queueing it.
func (e *Engine) CleanupNow(id string) error {
	j, err := e.Store.Get(context.Background(), id)
	if err != nil {
		return fmt.Errorf("read job before cleanup: %w", err)
	}
	if j.State != job.Imported && j.State != job.Failed {
		return errors.New("cleanup runs only for imported or failed jobs")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := e.cleanupFiles(ctx, j); err != nil {
		return fmt.Errorf("cleanup did not finish: %w", err)
	}
	_, err = e.Store.Update(context.Background(), id, func(c *job.Job) error {
		switch c.State {
		case job.Failed:
			c.FailCleaned, c.Attempts, c.NextRun = true, 0, time.Time{}
		case job.Imported:
			c.Move(job.Cleaned, "cleaned up by admin; staging, complete and client items confirmed gone", e.Now())
		}
		return nil
	})
	return err
}

func (e *Engine) MarkImported(id string) error {
	_, err := e.Store.Update(context.Background(), id, func(j *job.Job) error {
		if j.State != job.Ready {
			return errors.New("only a ready job can be marked imported")
		}
		j.Flag = ""
		j.Move(job.Imported, "marked imported by admin", e.Now())
		return nil
	})
	return err
}

// ------------------------------------------------------------------ loop

func (e *Engine) Run(ctx context.Context, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		e.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (e *Engine) Tick(ctx context.Context) {
	jobs, err := e.Store.List(ctx)
	if err != nil {
		e.Log.Printf("job loop stopped: repository read failed: %v", err)
		return
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		// A job keeps moving through consecutive states in one tick, so a finished
		// download is staged and handed back without waiting for the next tick.
		for hops := 0; hops < 8; hops++ {
			if !j.NextRun.IsZero() && e.Now().Before(j.NextRun) {
				break
			}
			e.step(ctx, j)
			nj, err := e.Store.Get(ctx, j.ID)
			if err != nil {
				e.Log.Printf("job loop stopped: repository read failed: %v", err)
				return
			}
			if nj.State == j.State {
				break
			}
			j = nj
		}
	}
}

func (e *Engine) step(ctx context.Context, j job.Job) {
	switch j.State {
	case job.Received:
		if j.Source == "reencode" {
			e.doReencodeStage(ctx, j)
			return
		}
		e.doSubmit(ctx, j)
	case job.Submitted, job.Downloading:
		e.doDownload(ctx, j)
	case job.Downloaded:
		if j.Source == "reencode" {
			e.sendToTdarr(j)
			return
		}
		e.doDownloaded(ctx, j)
	case job.Pulling:
		if j.Source == "reencode" {
			e.doReencodeStage(ctx, j)
			return
		}
		e.doPulling(ctx, j)
	case job.QueuedTranscode, job.Transcoding:
		e.doTranscode(ctx, j)
	case job.Verifying:
		e.doVerify(ctx, j)
	case job.Ready:
		e.doReady(ctx, j)
	case job.Imported:
		e.doCleanup(ctx, j)
	case job.Failed:
		e.doFailed(ctx, j)
	}
}

// retry records a transient error and schedules the next attempt, failing the
// job after max attempts.
func (e *Engine) retry(j job.Job, err error, max int, what string) {
	e.Log.Printf("job %s %s: %v", short(j.ID), what, err)
	var perm backend.Permanent
	if errors.As(err, &perm) {
		e.fail(j, perm.Msg)
		return
	}
	if errors.Is(err, backend.ErrBusy) {
		_, _ = e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
			c.Reason = err.Error() + " (waiting)"
			c.NextRun = e.Now().Add(time.Minute)
			c.Updated = e.Now()
			return nil
		})
		return
	}
	if j.Attempts+1 >= max {
		e.fail(j, fmt.Sprintf("%s failed: %v", what, err))
		return
	}
	wait := time.Duration(15*(j.Attempts+1)) * time.Second
	_, _ = e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
		c.Attempts++
		c.Reason = fmt.Sprintf("%s: %v (retrying)", what, err)
		c.NextRun = e.Now().Add(wait)
		c.Updated = e.Now()
		return nil
	})
}

func (e *Engine) fail(j job.Job, reason string) {
	e.Log.Printf("job %s failed: %s", short(j.ID), reason)
	_, _ = e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
		if c.State == job.Cleaned {
			return nil
		}
		c.Move(job.Failed, reason, e.Now())
		return nil
	})
}

func (e *Engine) doSubmit(ctx context.Context, j job.Job) {
	if e.Cfg.Get().IntakePaused {
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Reason = "intake is paused"
			c.NextRun = e.Now().Add(15 * time.Second)
			return nil
		})
		return
	}
	b, err := e.pick(j)
	if err != nil {
		e.retry(j, err, 4, "choose backend")
		return
	}
	rid, save, err := b.Add(ctx, j)
	if err != nil {
		e.retry(j, err, 4, "submit to client")
		return
	}
	_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
		c.Backend, c.RemoteID, c.SavePath = b.Name(), rid, save
		c.LastProgress = e.Now()
		c.Move(job.Submitted, "sent to "+c.Backend, e.Now())
		return nil
	})
}

// inactiveGrace is how long an inactive, motionless TorBox item is given before it is retried.
const inactiveGrace = 10 * time.Minute

// stuckGrace is how long a TorBox item may sit at 100% without files (unpack, repair) before it fails.
const stuckGrace = 15 * time.Minute

func (e *Engine) doDownload(ctx context.Context, j job.Job) {
	it, err := e.backend(j).Status(ctx, j.RemoteID)
	if err != nil {
		e.retry(j, err, 40, "client status")
		return
	}
	now := e.Now()
	if j.Attempts > 0 || strings.HasSuffix(j.Reason, "(retrying)") {
		// A good answer ends the retry streak and its stale message.
		j, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.Attempts, c.Reason = 0, ""; return nil })
	}
	switch it.Status {
	case backend.Missing:
		if j.State == job.Submitted && now.Sub(j.StateChanged) < 2*time.Minute {
			return
		}
		e.fail(j, "the download client no longer has this item (TorBox may have removed a failed download)")
	case backend.Errored:
		e.failStalled(ctx, j, it.Error)
	case backend.Complete:
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Progress = 1
			c.Move(job.Downloaded, "download finished", now)
			return nil
		})
	default:
		stall := time.Duration(e.Cfg.Get().StallMinutes) * time.Minute
		moved := it.Progress > j.Progress
		// TorBox showing the item as inactive with nothing moving is a failure on its
		// side ("Inactive / Processing"); do not wait the full stall time for it.
		if !moved && it.Stuck && now.Sub(j.LastProgress) > stuckGrace {
			e.failStalled(ctx, j, fmt.Sprintf("TorBox reached 100%% but did not finish within %d minutes", int(stuckGrace.Minutes())))
			return
		}
		if !moved && it.Inactive && now.Sub(j.LastProgress) > inactiveGrace {
			e.failStalled(ctx, j, fmt.Sprintf("TorBox shows it inactive with no progress for %d minutes", int(inactiveGrace.Minutes())))
			return
		}
		if !moved && now.Sub(j.LastProgress) > stall {
			e.failStalled(ctx, j, fmt.Sprintf("no progress for %d minutes", int(stall.Minutes())))
			return
		}
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			if moved {
				c.Progress, c.LastProgress = it.Progress, now
			}
			if c.State == job.Submitted {
				c.Move(job.Downloading, "client is downloading", now)
			}
			c.Updated = now
			return nil
		})
	}
}

// failStalled ends a download that errored or stopped moving. It is not tried
// again: the job fails, the cleanup removes the item from the client, and the arr
// sees the failure, blocklists the release and grabs a different one.
func (e *Engine) failStalled(ctx context.Context, j job.Job, why string) {
	if err := e.backend(j).Remove(ctx, j.RemoteID, true); err != nil {
		e.Log.Printf("job %s: deleting client item %s: %v (cleanup tries again)", short(j.ID), j.RemoteID, err)
	}
	e.fail(j, "download failed: "+why)
}

// ---------------------------------------------------- stage and hand back

var mediaExt = map[string]bool{
	".mkv": true, ".mp4": true, ".m4v": true, ".avi": true, ".mov": true, ".ts": true, ".m2ts": true, ".wmv": true, ".webm": true,
	".flac": true, ".mp3": true, ".m4a": true, ".aac": true, ".ogg": true, ".opus": true, ".wav": true, ".alac": true, ".ape": true, ".wv": true,
	".srt": true, ".ass": true, ".ssa": true, ".sub": true, ".idx": true, ".vtt": true, ".sup": true,
}

func wanted(f backend.File) bool {
	if !mediaExt[strings.ToLower(filepath.Ext(f.Path))] {
		return false
	}
	base := strings.ToLower(filepath.Base(f.Path))
	if strings.Contains(base, "sample") && f.Size < 200<<20 {
		return false
	}
	for _, part := range strings.Split(strings.ToLower(filepath.ToSlash(filepath.Dir(f.Path))), "/") {
		if part == "sample" || part == "samples" {
			return false
		}
	}
	return true
}

func (e *Engine) doDownloaded(ctx context.Context, j job.Job) {
	b := e.backend(j)
	staging := e.Cfg.Env.StagingDir(j.ID)
	if _, remote := b.(backend.Fetcher); remote && len(j.Files) == 0 {
		e.planPull(ctx, j, b)
		return
	}
	if loc, ok := b.(backend.Locator); ok && j.SavePath == "" {
		dir, err := loc.Location(ctx, j.RemoteID)
		if err != nil {
			e.retry(j, err, 10, "locate files")
			return
		}
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.SavePath = dir; return nil })
		j.SavePath = dir
	}
	if len(j.Files) == 0 {
		files, err := b.Files(ctx, j.RemoteID)
		if err != nil {
			e.retry(j, err, 10, "list files")
			return
		}
		var picked []backend.File
		for _, f := range files {
			if wanted(f) {
				picked = append(picked, f)
			}
		}
		if len(picked) == 0 {
			e.fail(j, "no media files in the download")
			return
		}
		strip := commonTop(picked)
		var rel []string
		for _, f := range picked {
			r := filepath.FromSlash(f.Path)
			if strip != "" {
				r = strings.TrimPrefix(r, strip+string(filepath.Separator))
			}
			src := filepath.Join(j.SavePath, filepath.FromSlash(f.Path))
			dst := filepath.Join(staging, r)
			if err := moveFile(src, dst); err != nil {
				e.retry(j, err, 6, "move to staging")
				return
			}
			st, err := os.Stat(dst)
			if err != nil || (f.Size > 0 && st.Size() != f.Size) {
				e.fail(j, "incomplete download: size mismatch on "+r)
				return
			}
			rel = append(rel, r)
		}
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.StagingDir, c.Files, c.Updated = staging, rel, e.Now()
			return nil
		})
		j.StagingDir, j.Files = staging, rel
	}
	// The local copy is verified: remove the client item and everything it left.
	// The files are safe in staging, so a client that is slow to confirm the delete
	// must not hold the job up or fail it; the final cleanup removes the item and
	// retries until it is confirmed gone.
	if err := e.removeBackendItem(ctx, j); err != nil {
		e.Log.Printf("job %s: client item not confirmed removed yet (%v); continuing, cleanup will retry", short(j.ID), err)
	}
	if why := e.qualityMismatch(ctx, j); why != "" {
		e.fail(j, why)
		return
	}
	cat, _ := e.Cfg.Get().Category(j.Category)
	if cat.Transcode && e.sendToTdarr(j) {
		return
	}
	e.handBack(j)
}

func commonTop(files []backend.File) string {
	var top string
	for i, f := range files {
		parts := strings.Split(filepath.ToSlash(f.Path), "/")
		if len(parts) < 2 {
			return ""
		}
		if i == 0 {
			top = parts[0]
		} else if parts[0] != top {
			return ""
		}
	}
	return top
}

func (e *Engine) handBack(j job.Job) {
	// The last look before the arr sees the files: this is the file that will be imported,
	// so a release that is smaller than it was grabbed as (or a transcode that scaled it
	// down) stops here.
	if why := e.qualityMismatch(context.Background(), j); why != "" {
		e.fail(j, why)
		return
	}
	root := e.Cfg.Env.CompleteRoot(j.Category)
	dest := filepath.Join(root, config.SafeName(j.Name))
	if _, err := os.Stat(dest); err == nil {
		dest += "." + short(j.ID)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		e.retry(j, err, 6, "create complete folder")
		return
	}
	if err := os.Rename(j.StagingDir, dest); err != nil {
		e.retry(j, err, 6, "move to complete")
		return
	}
	e.openUpLog(dest)
	_, _ = e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
		c.CompleteDir, c.StagingDir = dest, ""
		c.Move(job.Ready, "moved to "+dest, e.Now())
		return nil
	})
	e.Log.Printf("job %s ready at %s", short(j.ID), dest)
}

// ------------------------------------------------------- import and clean

func (e *Engine) doReady(ctx context.Context, j job.Job) {
	now := e.Now()
	if j.Source == "reencode" {
		e.doReencodeSwap(ctx, j)
		return
	}
	// RSS and watch-folder jobs were not grabbed by an arr, so none will import
	// them: the finished files stay where they were delivered and the job ends.
	if j.Source == "rss" || j.Source == "watch" {
		if cat, ok := e.Cfg.Get().Category(j.Category); ok && cat.LibraryDir != "" {
			e.placeInLibrary(ctx, j, cat)
			return
		}
		if err := e.cleanupExcept(ctx, j, true); err != nil {
			e.retry2(j, err)
			return
		}
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Move(job.Cleaned, "delivered to "+c.CompleteDir, now)
			return nil
		})
		e.Log.Printf("job %s delivered to %s", short(j.ID), j.CompleteDir)
		return
	}
	if j.ArrDeleted {
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Move(job.Imported, "the arr removed the item after import", now)
			return nil
		})
		return
	}
	s := e.Cfg.Get()
	cat, _ := s.Category(j.Category)
	next := now.Add(time.Duration(s.ReconcileSeconds) * time.Second)
	if chk := e.arrFor(cat); chk != nil {
		ok, err := chk.Imported(ctx, j.DownloadID())
		switch {
		case err != nil:
			e.Log.Printf("job %s import check: %v", short(j.ID), err)
		case ok:
			_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
				c.Move(job.Imported, "import event found in the arr's history", now)
				return nil
			})
			return
		default:
			if n, has := chk.(interface{ Note() string }); has {
				e.noteOnce(j.ID, "import check: "+n.Note())
			}
		}
	}
	// An arr that imports by moving takes the files out of the complete folder. If
	// none are left a while after the job was ready, the import happened even when
	// the history call did not show it.
	if j.CompleteDir != "" && now.Sub(j.StateChanged) > time.Minute && !hasMedia(j.CompleteDir) {
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Move(job.Imported, "the files left the complete folder, so the arr moved them", now)
			return nil
		})
		e.Log.Printf("job %s: no media left in %s, treating it as imported", short(j.ID), j.CompleteDir)
		return
	}
	flag := ""
	if h := int(now.Sub(j.StateChanged).Hours()); h >= s.FlagImportHours {
		flag = fmt.Sprintf("waiting for the arr to import for %d hours", h)
	}
	_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.NextRun, c.Flag = next, flag; return nil })
}

func (e *Engine) removeBackendItem(ctx context.Context, j job.Job) error {
	if j.RemoteID == "" && len(j.PastRemoteIDs) == 0 {
		return nil // nothing was ever sent to a client (a re-encode job, or a grab that never got that far)
	}
	b := e.backend(j)
	// Items from earlier attempts: delete them, and do not hold up cleanup if one is already gone.
	for _, old := range j.PastRemoteIDs {
		if old == "" || old == j.RemoteID {
			continue
		}
		if err := b.Remove(ctx, old, true); err != nil {
			e.Log.Printf("job %s: removing earlier client item %s: %v", short(j.ID), old, err)
		}
	}
	if j.RemoteID != "" {
		if it, err := b.Status(ctx, j.RemoteID); err != nil {
			return err
		} else if it.Status == backend.Missing {
			// A failed item can drop out of TorBox's list while it still sits in the
			// account. Ask for the delete anyway; "not found" is the expected answer.
			if err := b.Remove(ctx, j.RemoteID, true); err != nil {
				e.Log.Printf("job %s: client item %s not listed; delete answered: %v", short(j.ID), j.RemoteID, err)
			} else {
				e.Log.Printf("job %s: client item %s was not listed but has been deleted", short(j.ID), j.RemoteID)
			}
		} else {
			if err := b.Remove(ctx, j.RemoteID, true); err != nil {
				return err
			}
			// Some clients take a few seconds to drop a deleted item from their list.
			gone := false
			for i := 0; i < 4 && !gone; i++ {
				if i > 0 {
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-time.After(2 * time.Second):
					}
				}
				st, err := b.Status(ctx, j.RemoteID)
				if err != nil {
					return err
				}
				gone = st.Status == backend.Missing
			}
			if !gone {
				return fmt.Errorf("the client still lists item %s after it was deleted", j.RemoteID)
			}
		}
	}
	if j.SavePath != "" {
		root := e.Cfg.Get().QBit.SaveDir
		if j.Backend == "nzbget" {
			root = e.Cfg.Get().NZBGet.DestRoot
		}
		if err := e.removeTree(j.SavePath, root); err != nil {
			return err
		}
	}
	if j.NZBPath != "" {
		if err := os.Remove(j.NZBPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	if j.TorrentPath != "" {
		if err := os.Remove(j.TorrentPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// removeTree deletes p only when it sits inside root, and verifies it is gone.
func (e *Engine) removeTree(p, root string) error {
	if p == "" {
		return nil
	}
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("refusing to delete %s: outside %s", p, root)
	}
	if err := os.RemoveAll(p); err != nil {
		return err
	}
	if _, err := os.Stat(p); err == nil {
		return fmt.Errorf("%s still exists", p)
	}
	return nil
}

func (e *Engine) cleanupFiles(ctx context.Context, j job.Job) error {
	return e.cleanupExcept(ctx, j, false)
}

// cleanupExcept removes what a job left behind. With keepDelivered the finished
// files in the complete folder stay (jobs no arr is waiting to import).
func (e *Engine) cleanupExcept(ctx context.Context, j job.Job, keepDelivered bool) error {
	env := e.Cfg.Env
	if !keepDelivered {
		if err := e.removeTree(j.CompleteDir, env.Complete()); err != nil {
			return err
		}
	}
	if err := e.removeTree(env.StagingDir(j.ID), env.Work()); err != nil {
		return err
	}
	if err := e.removeTdarrDirs(j); err != nil {
		return err
	}
	return e.removeBackendItem(ctx, j)
}

func (e *Engine) doCleanup(ctx context.Context, j job.Job) {
	if err := e.cleanupFiles(ctx, j); err != nil {
		e.retry2(j, err)
		return
	}
	_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
		c.Move(job.Cleaned, "staging, complete and client items confirmed gone", e.Now())
		return nil
	})
	e.Log.Printf("job %s cleaned", short(j.ID))
}

// retry2 backs off cleanup forever: it is retried until it succeeds.
func (e *Engine) retry2(j job.Job, err error) {
	e.Log.Printf("job %s cleanup: %v", short(j.ID), err)
	wait := time.Duration(30*(j.Attempts+1)) * time.Second
	if wait > 10*time.Minute {
		wait = 10 * time.Minute
	}
	_, _ = e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
		c.Attempts++
		if c.State != job.Failed { // a failed job keeps the reason it failed
			c.Reason = "cleanup: " + err.Error() + " (retrying)"
		}
		c.NextRun = e.Now().Add(wait)
		c.Updated = e.Now()
		return nil
	})
}

func (e *Engine) doFailed(ctx context.Context, j job.Job) {
	if !j.FailCleaned {
		if err := e.cleanupFiles(ctx, j); err != nil {
			e.retry2(j, err)
			return
		}
		j.FailCleaned = true
		_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error { c.FailCleaned, c.Attempts, c.NextRun = true, 0, time.Time{}; return nil })
	}
	// Do not wait for the arr's own failed-download handling (it may be switched
	// off, or the item may already be gone from its queue): tell it directly, so it
	// blocklists this release and grabs a different one, then end the job.
	if j.ArrDeleted || j.Source == "rss" || j.Source == "watch" || j.Source == "reencode" {
		e.endFailed(j, "failed job cleaned up; the arr has dealt with it")
		return
	}
	cat, _ := e.Cfg.Get().Category(j.Category)
	f, ok := e.arrFor(cat).(DownloadFailer)
	if !ok {
		e.endFailed(j, "failed job cleaned up; no arr is set for this category")
		return
	}
	found, err := f.FailDownload(ctx, j.DownloadID())
	if err != nil {
		// Stay visible as failed so the arr's own handling can still see it, but do
		// not sit forever: after six hours the job ends.
		if e.Now().Sub(j.StateChanged) > 6*time.Hour {
			e.endFailed(j, "failed job cleaned up; the arr could not be told: "+err.Error())
			return
		}
		e.retry2(j, fmt.Errorf("telling the arr: %w", err))
		return
	}
	if found {
		e.Log.Printf("job %s: the arr was told to blocklist and search again", short(j.ID))
		e.endFailed(j, "failed; the arr blocklisted the release and will search again")
		return
	}
	e.endFailed(j, "failed job cleaned up; the arr no longer had it in its queue")
}

func (e *Engine) endFailed(j job.Job, why string) {
	_, _ = e.Store.Update(context.Background(), j.ID, func(c *job.Job) error {
		c.ArrNotified = true
		c.Attempts, c.NextRun = 0, time.Time{}
		c.Move(job.Cleaned, why, e.Now())
		return nil
	})
}

// ---------------------------------------------------------------- files

func moveFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	err := os.Rename(src, dst)
	if err == nil {
		return nil
	}
	var le *os.LinkError
	if !(errors.As(err, &le) && errors.Is(le.Err, syscall.EXDEV)) {
		if _, serr := os.Stat(dst); serr == nil {
			if _, serr2 := os.Stat(src); errors.Is(serr2, fs.ErrNotExist) {
				return nil // an earlier attempt already moved it
			}
		}
		return err
	}
	// Different filesystems: copy, verify the size, then remove the source.
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, in)
	if err == nil {
		err = out.Sync()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if st, err := os.Stat(src); err != nil || st.Size() != n {
		return errors.New("copy size mismatch")
	}
	return os.Remove(src)
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// ------------------------------------------------------------- remote pull

// planPull lists the remote files, keeps the media, and moves the job to Pulling.
func (e *Engine) planPull(ctx context.Context, j job.Job, b backend.Backend) {
	files, err := b.Files(ctx, j.RemoteID)
	if err != nil {
		e.retry(j, err, 10, "list files")
		return
	}
	var picked []backend.File
	var total int64
	for _, f := range files {
		if wanted(f) {
			picked = append(picked, f)
			total += f.Size
		}
	}
	if len(picked) == 0 {
		e.fail(j, "no media files in the download")
		return
	}
	_, _ = e.Store.Update(ctx, j.ID, func(c *job.Job) error {
		c.PullTotal, c.PullDone, c.LastProgress = total, 0, e.Now()
		c.Move(job.Pulling, fmt.Sprintf("copying %d file(s) from %s", len(picked), c.Backend), e.Now())
		return nil
	})
}

// doPulling makes sure a background copy is running; it is idempotent, so it
// also restarts the copy after a restart of the container.
func (e *Engine) doPulling(ctx context.Context, j job.Job) {
	if _, loaded := e.pulling.LoadOrStore(j.ID, true); loaded {
		return
	}
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		defer e.pulling.Delete(j.ID)
		select {
		case e.sem <- struct{}{}:
			defer func() { <-e.sem }()
		case <-ctx.Done():
			return
		}
		e.runPull(ctx, j.ID)
	}()
}

func (e *Engine) runPull(ctx context.Context, id string) {
	j, err := e.Store.Get(ctx, id)
	if err != nil {
		e.Log.Printf("pull stopped: repository read failed: %v", err)
		return
	}
	if j.State != job.Pulling {
		return
	}
	b := e.backend(j)
	f, ok := b.(backend.Fetcher)
	if !ok {
		e.fail(j, "backend cannot copy files")
		return
	}
	files, err := b.Files(ctx, j.RemoteID)
	if err != nil {
		e.retry(j, err, 10, "list files")
		return
	}
	var picked []backend.File
	for _, rf := range files {
		if wanted(rf) {
			picked = append(picked, rf)
		}
	}
	strip := commonTop(picked)
	staging := e.Cfg.Env.StagingDir(j.ID)
	var done int64
	var rel []string
	var lastSave time.Time
	for _, rf := range picked {
		r := filepath.FromSlash(rf.Path)
		if strip != "" {
			r = strings.TrimPrefix(r, strip+string(filepath.Separator))
		}
		r = filepath.Clean(r)
		if filepath.IsAbs(r) || strings.HasPrefix(r, "..") {
			e.fail(j, "unsafe file path from the backend: "+rf.Path)
			return
		}
		dst := filepath.Join(staging, r)
		if st, err := os.Stat(dst); err == nil && (rf.Size == 0 || st.Size() == rf.Size) {
			done += st.Size()
			rel = append(rel, r)
			continue
		}
		base := done
		var got int64
		err := f.Fetch(ctx, j.RemoteID, rf, dst, func(n int64) {
			got += n
			if now := e.Now(); now.Sub(lastSave) > 2*time.Second {
				lastSave = now
				_, _ = e.Store.Update(ctx, id, func(c *job.Job) error {
					c.PullDone, c.LastProgress, c.Updated = base+got, now, now
					return nil
				})
			}
		})
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			cur, readErr := e.Store.Get(ctx, id)
			if readErr != nil {
				e.Log.Printf("pull retry stopped: repository read failed: %v", readErr)
				return
			}
			e.retry(cur, err, 8, "copy from "+j.Backend)
			return
		}
		done += rf.Size
		rel = append(rel, r)
	}
	_, _ = e.Store.Update(ctx, id, func(c *job.Job) error {
		if c.State != job.Pulling {
			return nil
		}
		c.StagingDir, c.Files, c.PullDone = staging, rel, c.PullTotal
		c.Move(job.Downloaded, "files copied", e.Now())
		return nil
	})
}

// noteOnce logs a line for a job only when it differs from the last one, so a
// check that repeats every few seconds does not fill the log.
func (e *Engine) noteOnce(id, msg string) {
	e.nmu.Lock()
	defer e.nmu.Unlock()
	if e.notes == nil {
		e.notes = map[string]string{}
	}
	if e.notes[id] == msg {
		return
	}
	e.notes[id] = msg
	e.Log.Printf("job %s %s", short(id), msg)
}

// hasMedia reports whether any media file is left under dir.
func hasMedia(dir string) bool {
	found := false
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if !d.IsDir() && mediaExt[strings.ToLower(filepath.Ext(d.Name()))] {
			found = true
		}
		return nil
	})
	return found
}
