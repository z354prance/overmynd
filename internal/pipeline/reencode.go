package pipeline

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/media"
)

// Re-encoding runs a file that is already in the library through the same Tdarr
// pipeline as a new download. The library file is only replaced once Tdarr's output
// has passed the usual checks, has been read back as AV1 and has been written next to
// the original in full; until then the original is untouched, and it stays untouched if
// anything fails.

// inLibrary reports whether path is a file inside one of the media roots.
func (e *Engine) inLibrary(path string) bool {
	for _, r := range e.LibraryRoots {
		if r == "" {
			continue
		}
		if rel, err := filepath.Rel(r, path); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}

// ReencodeID is the job ID for a library file, so the same file is never queued twice.
func ReencodeID(path string) string {
	sum := sha1.Sum([]byte(filepath.Clean(path)))
	return "reencode_" + hex.EncodeToString(sum[:])[:16]
}

// SubmitReencode queues one library file for re-encoding. It returns the existing job
// when the same file is already being worked on.
func (e *Engine) SubmitReencode(path string) (job.Job, error) {
	if len(e.LibraryRoots) == 0 {
		return job.Job{}, errors.New("no media library is set (UNDERMYND_STORAGE_ROOT)")
	}
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || !e.inLibrary(clean) {
		return job.Job{}, errors.New("not a file in the media library")
	}
	if !media.IsVideo(clean) {
		return job.Job{}, errors.New("not a video file")
	}
	fi, err := os.Lstat(clean)
	if err != nil || !fi.Mode().IsRegular() {
		return job.Job{}, errors.New("not a regular file")
	}
	id := ReencodeID(clean)
	e.submitMu.Lock()
	defer e.submitMu.Unlock()
	if cur, ok := e.Store.Get(id); ok && cur.State != job.Cleaned && cur.State != job.Failed {
		return cur, nil
	}
	now := e.Now()
	j := job.Job{ID: id, Name: filepath.Base(clean), Category: "reencode", Kind: "local", Source: "reencode",
		State: job.Received, Size: fi.Size(), Original: clean, OriginalSize: fi.Size(), OriginalMod: fi.ModTime().UnixNano(),
		Created: now, Updated: now, StateChanged: now, LastProgress: now}
	j.History = []job.Transition{{At: now, To: job.Received, Reason: "queued for re-encoding to AV1"}}
	if err := e.Store.Put(j); err != nil {
		return job.Job{}, err
	}
	e.Log.Printf("job %s queued: re-encode %s", short(id), clean)
	return j, nil
}

func (e *Engine) doReencodeStage(ctx context.Context, j job.Job) {
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
		e.runReencodeStage(ctx, j.ID)
	}()
}

// libraryFileUnchanged checks the library file is still the one that was queued.
func libraryFileUnchanged(j job.Job) error {
	fi, err := os.Stat(j.Original)
	if err != nil {
		return fmt.Errorf("the library file is gone: %w", err)
	}
	if fi.Size() != j.OriginalSize || fi.ModTime().UnixNano() != j.OriginalMod {
		return errors.New("the library file changed after it was queued, so it was left alone")
	}
	return nil
}

type countWriter struct {
	w    io.Writer
	n    int64
	tick func(n int64)
	last time.Time
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	if time.Since(c.last) > 2*time.Second {
		c.last = time.Now()
		c.tick(c.n)
	}
	return n, err
}

// copyFileSynced copies src to dst, reporting progress, and syncs it to disk.
func copyFileSynced(ctx context.Context, src, dst string, tick func(int64)) (int64, error) {
	in, err := os.Open(src)
	if err != nil {
		return 0, err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o666)
	if err != nil {
		return 0, err
	}
	cw := &countWriter{w: out, tick: tick}
	buf := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			out.Close()
			return cw.n, err
		}
		n, rerr := in.Read(buf)
		if n > 0 {
			if _, werr := cw.Write(buf[:n]); werr != nil {
				out.Close()
				return cw.n, werr
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			return cw.n, rerr
		}
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return cw.n, err
	}
	return cw.n, out.Close()
}

func (e *Engine) runReencodeStage(ctx context.Context, id string) {
	j, ok := e.Store.Get(id)
	if !ok || (j.State != job.Received && j.State != job.Pulling) {
		return
	}
	if err := libraryFileUnchanged(j); err != nil {
		e.fail(j, err.Error())
		return
	}
	staging := e.Cfg.Env.StagingDir(id)
	if err := e.removeTree(staging, e.Cfg.Env.Work()); err != nil {
		e.retry(j, err, 6, "clear staging")
		return
	}
	if err := os.MkdirAll(staging, 0o755); err != nil {
		e.retry(j, err, 6, "create staging")
		return
	}
	_, _ = e.Store.Update(id, func(c *job.Job) error {
		c.PullTotal, c.PullDone = c.OriginalSize, 0
		c.Move(job.Pulling, "copying from the library", e.Now())
		return nil
	})
	base := filepath.Base(j.Original)
	tmp, dst := filepath.Join(staging, ".part"), filepath.Join(staging, base)
	n, err := copyFileSynced(ctx, j.Original, tmp, func(done int64) {
		_, _ = e.Store.Update(id, func(c *job.Job) error { c.PullDone, c.LastProgress = done, e.Now(); return nil })
	})
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		e.retry(j, err, 6, "copy from the library")
		return
	}
	if n != j.OriginalSize {
		e.fail(j, "the copy from the library is not the same size as the original")
		return
	}
	if err := libraryFileUnchanged(j); err != nil {
		e.fail(j, err.Error())
		return
	}
	if err := os.Rename(tmp, dst); err != nil {
		e.retry(j, err, 6, "finish the copy")
		return
	}
	_, _ = e.Store.Update(id, func(c *job.Job) error {
		c.StagingDir, c.Files, c.PullDone = staging, []string{base}, n
		c.LastProgress = e.Now()
		c.Move(job.Downloaded, "copied to staging", e.Now())
		return nil
	})
}

// doReencodeSwap replaces the library file with Tdarr's verified output.
func (e *Engine) doReencodeSwap(ctx context.Context, j job.Job) {
	if len(j.Files) != 1 {
		e.fail(j, fmt.Sprintf("expected one output file, found %d", len(j.Files)))
		return
	}
	src := filepath.Join(j.CompleteDir, j.Files[0])
	newPath := filepath.Join(filepath.Dir(j.Original), stem(filepath.Base(j.Original))+filepath.Ext(src))

	// A restart between the swap and the state change leaves the work already done.
	if _, err := os.Stat(src); err != nil {
		if c, cerr := media.VideoCodec(newPath); cerr == nil && c == "av1" {
			if _, oerr := os.Stat(j.Original); oerr != nil || newPath == j.Original {
				e.finishReencode(ctx, j, "replaced with AV1")
				return
			}
		}
		e.fail(j, "Tdarr's output is missing: "+err.Error())
		return
	}
	if err := libraryFileUnchanged(j); err != nil {
		e.fail(j, err.Error())
		return
	}
	codec, err := media.VideoCodec(src)
	if err != nil || codec != "av1" {
		got := codec
		if err != nil {
			got = "unreadable (" + err.Error() + ")"
		}
		e.fail(j, "Tdarr's output is not AV1 (it is "+got+"); the library file was left alone")
		return
	}
	if newPath != j.Original {
		if _, err := os.Lstat(newPath); err == nil {
			e.fail(j, "a different file already exists at "+newPath+"; the library file was left alone")
			return
		}
	}
	// Write the whole new file beside the original first; nothing in the library
	// changes until it is complete and read back.
	part := filepath.Join(filepath.Dir(newPath), ".undermynd-"+short(j.ID)+".part")
	_, _ = e.Store.Update(j.ID, func(c *job.Job) error { c.Reason = "writing the AV1 file into the library"; return nil })
	n, err := copyFileSynced(ctx, src, part, func(int64) {})
	if err != nil || n != fileSize(src) {
		_ = os.Remove(part)
		if err == nil {
			err = errors.New("the copy is not the same size as Tdarr's output")
		}
		e.retry(j, err, 6, "write into the library")
		return
	}
	if c, cerr := media.VideoCodec(part); cerr != nil || c != "av1" {
		_ = os.Remove(part)
		e.fail(j, "the file written to the library did not read back as AV1; the library file was left alone")
		return
	}
	copyOwnerAndMode(j.Original, part)
	if err := os.Rename(part, newPath); err != nil {
		_ = os.Remove(part)
		e.retry(j, err, 6, "put the AV1 file in place")
		return
	}
	if newPath != j.Original {
		if err := os.Remove(j.Original); err != nil {
			e.Log.Printf("job %s: replaced %s but could not remove the old file: %v", short(j.ID), j.Original, err)
		}
	}
	e.finishReencode(ctx, j, fmt.Sprintf("replaced with AV1 (%s to %s)", humanBytes(j.OriginalSize), humanBytes(n)))
}

func (e *Engine) finishReencode(ctx context.Context, j job.Job, why string) {
	if err := e.cleanupExcept(ctx, j, false); err != nil {
		e.retry2(j, err)
		return
	}
	_, _ = e.Store.Update(j.ID, func(c *job.Job) error {
		c.Move(job.Cleaned, why, e.Now())
		return nil
	})
	e.Log.Printf("job %s: %s: %s", short(j.ID), j.Original, why)
}

func fileSize(p string) int64 {
	if fi, err := os.Stat(p); err == nil {
		return fi.Size()
	}
	return -1
}

func humanBytes(n int64) string {
	const g = 1 << 30
	if n >= g {
		return fmt.Sprintf("%.2f GB", float64(n)/g)
	}
	return fmt.Sprintf("%.0f MB", float64(n)/(1<<20))
}

// copyOwnerAndMode gives the new file the original's permissions and owner, as far as
// this process is allowed to.
func copyOwnerAndMode(from, to string) {
	fi, err := os.Stat(from)
	if err != nil {
		return
	}
	_ = os.Chmod(to, fi.Mode().Perm())
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		_ = os.Chown(to, int(st.Uid), int(st.Gid))
	}
}
