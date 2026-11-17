package pipeline

import (
	"io/fs"
	"path/filepath"
	"time"

	"github.com/z354prance/overmynd/internal/job"
)

type DiskUse struct {
	Path       string `json:"path"`
	FreeBytes  uint64 `json:"freeBytes"`
	TotalBytes uint64 `json:"totalBytes"`
	UsedByUs   int64  `json:"usedByUs"` // bytes in Overmynd's own folders (capped scan)
	Partial    bool   `json:"partial"`  // the scan stopped early
}

type SystemInfo struct {
	Paused bool           `json:"paused"`
	Counts map[string]int `json:"counts"` // jobs per public group
	TorBox struct {
		Configured   bool       `json:"configured"`
		Used         int        `json:"used"`
		Limit        int        `json:"limit"`
		BackoffUntil *time.Time `json:"backoffUntil,omitempty"`
	} `json:"torbox"`
	Tdarr struct {
		InFlight int  `json:"inFlight"`
		Max      int  `json:"max"`
		API      bool `json:"api"`
	} `json:"tdarr"`
	Work     DiskUse `json:"work"`
	Complete DiskUse `json:"complete"`
}

// dirSize adds up file sizes, stopping after a file count so a huge tree cannot stall the page.
func dirSize(root string) (n int64, partial bool) {
	files := 0
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if files++; files > 20000 {
				partial = true
				return fs.SkipAll
			}
			if info, err := d.Info(); err == nil {
				n += info.Size()
			}
		}
		return nil
	})
	return
}

func (e *Engine) disk(path string) DiskUse {
	d := DiskUse{Path: path}
	d.FreeBytes, d.TotalBytes = diskSpace(path)
	d.UsedByUs, d.Partial = dirSize(path)
	return d
}

// System gathers the numbers for the admin System view.
func (e *Engine) System() SystemInfo {
	var si SystemInfo
	s := e.Cfg.Get()
	si.Paused = s.IntakePaused
	si.Counts = map[string]int{}
	for _, j := range e.Store.List() {
		if j.State != job.Cleaned {
			si.Counts[job.Group(j.State)]++
		}
	}
	if tb := e.torbox(); tb != nil {
		si.TorBox.Configured = true
		e.bmu.Lock()
		lim := e.lim
		e.bmu.Unlock()
		if lim != nil {
			used, limit, until := lim.Status()
			si.TorBox.Used, si.TorBox.Limit = used, limit
			if until.After(e.Now()) {
				si.TorBox.BackoffUntil = &until
			}
		}
	}
	si.Tdarr.InFlight, si.Tdarr.Max, si.Tdarr.API = e.tdarrInFlight(), s.Tdarr.MaxInFlight, s.Tdarr.URL != ""
	si.Work = e.disk(e.Cfg.Env.Work())
	si.Complete = e.disk(e.Cfg.Env.Complete())
	return si
}
