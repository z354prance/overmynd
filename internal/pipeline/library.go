package pipeline

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/library"
)

// placeInLibrary moves each finished episode into its show's season folder, named
// like the rest of the library. An episode it cannot place with certainty is left
// where it was delivered, and nothing is ever overwritten. The job ends either way.
func (e *Engine) placeInLibrary(ctx context.Context, j job.Job, cat config.Category) {
	now := e.Now()
	root := cat.LibraryDir
	folders, err := library.Folders(root)
	if err != nil {
		e.retry(j, fmt.Errorf("library folder %s: %w", root, err), 10, "library")
		return
	}
	var left []string
	placed := 0
	for _, f := range j.Files {
		if !isVideo(f) {
			continue
		}
		src := filepath.Join(j.CompleteDir, f)
		if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
			continue // placed on an earlier pass
		}
		dst, why, err := placeOne(root, folders, src)
		if err != nil {
			e.retry(j, err, 10, "place in library")
			return
		}
		if why != "" {
			left = append(left, why)
			continue
		}
		placed++
		e.Log.Printf("job %s placed %s", short(j.ID), dst)
	}
	if len(left) == 0 {
		if err := e.cleanupExcept(ctx, j, false); err != nil {
			e.retry2(j, err)
			return
		}
		if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
			c.Flag = ""
			c.Move(job.Cleaned, fmt.Sprintf("placed %d file(s) in %s", placed, root), now)
			return nil
		}); writeErr != nil {
			e.Log.Printf("job state update failed: %v", writeErr)
			return
		}
		return
	}
	// Some files could not be placed: keep everything in the complete folder as it is.
	if err := e.cleanupExcept(ctx, j, true); err != nil {
		e.retry2(j, err)
		return
	}
	why := fmt.Sprintf("placed %d, left in %s: %s", placed, j.CompleteDir, strings.Join(dedupe(left), "; "))
	if _, writeErr := e.Store.Update(ctx, j.ID, func(c *job.Job) error {
		c.Move(job.Cleaned, why, now)
		return nil
	}); writeErr != nil {
		e.Log.Printf("job state update failed: %v", writeErr)
		return
	}
	e.Log.Printf("job %s %s", short(j.ID), why)
}

// placeOne moves one file into the library. It returns the new path, or a reason
// the file was left where it is (an error only when the move itself failed).
func placeOne(root string, folders []string, src string) (dst, why string, err error) {
	base := filepath.Base(src)
	rel, ok := library.Parse(base)
	if !ok {
		return "", base + ": the name has no episode number", nil
	}
	folder, cands := library.Match(rel.Title, folders)
	if folder == "" {
		msg := fmt.Sprintf("no folder named %q", rel.Title)
		if len(cands) > 0 {
			msg += " (closest: " + strings.Join(cands, ", ") + ")"
		}
		return "", msg, nil
	}
	dst = filepath.Join(library.SeasonDir(filepath.Join(root, folder), rel.Season), library.Clean(base))
	if _, serr := os.Stat(dst); serr == nil {
		return "", fmt.Sprintf("%s already exists in %s", filepath.Base(dst), folder), nil
	}
	if err := moveFile(src, dst); err != nil {
		return "", "", err
	}
	// Open up the show's season folder and the file, so the network share can manage them.
	_ = os.Chmod(dst, 0o666)
	_ = os.Chmod(filepath.Dir(dst), 0o777)
	return dst, "", nil
}

// SweepLibrary places the video files already sitting in a no-arr category's
// complete folder (delivered before a library folder was set). Files newer than two
// minutes are skipped, since a running job may still be handling them. Emptied
// folders are removed; anything unplaceable stays.
func (e *Engine) SweepLibrary(category string) (string, error) {
	cat, ok := e.Cfg.Get().Category(category)
	if !ok || cat.LibraryDir == "" {
		return "", errors.New("that category has no library folder")
	}
	folders, err := library.Folders(cat.LibraryDir)
	if err != nil {
		return "", fmt.Errorf("library folder %s: %w", cat.LibraryDir, err)
	}
	top := e.Cfg.Env.CompleteRoot(category)
	var files []string
	_ = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && isVideo(p) && !strings.HasPrefix(d.Name(), ".") {
			if fi, ierr := d.Info(); ierr == nil && e.Now().Sub(fi.ModTime()) > 2*time.Minute {
				files = append(files, p)
			}
		}
		return nil
	})
	e.openUpLog(top) // fixes what was delivered before, as far as this process owns it
	placed := 0
	var left []string
	for _, src := range files {
		dst, why, err := placeOne(cat.LibraryDir, folders, src)
		if err != nil {
			return "", err
		}
		if why != "" {
			left = append(left, why)
			continue
		}
		placed++
		e.Log.Printf("sweep placed %s", dst)
	}
	// Remove job folders that are now empty (deepest first); never the category folder.
	var dirs []string
	_ = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() && p != top {
			dirs = append(dirs, p)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i]) // fails harmlessly when not empty
	}
	msg := fmt.Sprintf("placed %d of %d file(s)", placed, len(files))
	if l := dedupe(left); len(l) > 0 {
		msg += "; left: " + strings.Join(l, "; ")
	}
	e.Log.Printf("sweep %s: %s", category, msg)
	return msg, nil
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
