// Package watch sends the .torrent, .nzb and .magnet files dropped into a folder
// through the same pipeline as every other download. Standard library only.
package watch

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/z354prance/undermynd/internal/config"
	"github.com/z354prance/undermynd/internal/job"
	"github.com/z354prance/undermynd/internal/pipeline"
)

const (
	Submitted = "submitted-to-undermynd"
	Rejected  = "rejected-by-undermynd"
	maxFile   = 20 << 20
	settle    = 3 * time.Second // a file changed more recently than this may still be written
)

// Submitter is what the poller needs from the engine.
type Submitter interface {
	Submit(pipeline.SubmitRequest) (job.Job, error)
	SubmitNZB(pipeline.SubmitNZBRequest) (job.Job, error)
}

type Poller struct {
	Cfg      *config.Config
	Sub      Submitter
	Log      *log.Logger
	Interval time.Duration
	Now      func() time.Time
	noCat    map[string]bool   // folders already reported as having no category
	last     map[string]string // the last status line logged per folder
}

func New(cfg *config.Config, sub Submitter, l *log.Logger) *Poller {
	return &Poller{Cfg: cfg, Sub: sub, Log: l, Interval: 10 * time.Second, Now: time.Now, noCat: map[string]bool{}, last: map[string]string{}}
}

func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		p.Scan()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (p *Poller) category(w config.Watch, s config.Settings) string {
	if w.Category != "" {
		return w.Category
	}
	for _, r := range s.Rules {
		if r.Enabled && r.Category != "" {
			return r.Category
		}
	}
	return ""
}

// Scan checks every enabled folder once.
func (p *Poller) Scan() {
	s := p.Cfg.Get()
	for _, w := range s.Watches {
		if !w.Enabled || w.Dir == "" {
			continue
		}
		cat := p.category(w, s)
		if cat == "" {
			if !p.noCat[w.Dir] {
				p.noCat[w.Dir] = true
				p.Log.Printf("watch %s: no category to use; choose one for this folder or add an RSS rule", w.Dir)
			}
			continue
		}
		delete(p.noCat, w.Dir)
		p.scanDir(w.Dir, cat)
	}
}

// status logs a folder's state only when it changes, so a quiet folder stays quiet.
func (p *Poller) status(dir, msg string) {
	if p.last == nil {
		p.last = map[string]string{}
	}
	if p.last[dir] == msg {
		return
	}
	p.last[dir] = msg
	p.Log.Printf("watch %s: %s", dir, msg)
}

func (p *Poller) scanDir(dir, cat string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		p.status(dir, "cannot read the folder: "+err.Error())
		return
	}
	candidates, ignored := 0, 0
	for _, e := range ents {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".torrent" && ext != ".nzb" && ext != ".magnet" {
			ignored++
			continue
		}
		candidates++
		info, err := e.Info()
		if err != nil || !info.Mode().IsRegular() || p.Now().Sub(info.ModTime()) < settle {
			continue
		}
		p.one(dir, name, ext, cat, info.Size())
	}
	p.status(dir, fmt.Sprintf("watching with category %q: %d torrent/nzb/magnet file(s) found, %d other file(s) ignored", cat, candidates, ignored))
}

func (p *Poller) one(dir, name, ext, cat string, size int64) {
	full := filepath.Join(dir, name)
	if size > maxFile || size == 0 {
		p.move(dir, full, Rejected, fmt.Sprintf("size %d is not a usable file", size))
		return
	}
	f, err := os.Open(full)
	if err != nil {
		return // try again next pass
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFile+1))
	f.Close()
	if err != nil {
		return
	}
	var jb job.Job
	switch ext {
	case ".torrent":
		jb, err = p.Sub.Submit(pipeline.SubmitRequest{Source: "watch", Category: cat, Torrent: data})
	case ".nzb":
		jb, err = p.Sub.SubmitNZB(pipeline.SubmitNZBRequest{Source: "watch", Category: cat, Name: strings.TrimSuffix(name, filepath.Ext(name)), NZB: data})
	default:
		mag := strings.TrimSpace(string(data))
		if i := strings.Index(mag, "magnet:"); i >= 0 {
			mag = strings.Fields(mag[i:])[0]
		}
		jb, err = p.Sub.Submit(pipeline.SubmitRequest{Source: "watch", Category: cat, Magnet: mag})
	}
	if err != nil {
		p.move(dir, full, Rejected, err.Error())
		return
	}
	p.Log.Printf("watch %s: %s sent as job %s", dir, name, short(jb.ID))
	p.move(dir, full, Submitted, "")
}

func short(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// move puts a handled file into a subfolder so it is not picked up twice.
func (p *Poller) move(dir, full, sub, why string) {
	if why != "" {
		p.Log.Printf("watch %s: %s refused: %s", dir, filepath.Base(full), why)
	}
	dstDir := filepath.Join(dir, sub)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		p.Log.Printf("watch %s: cannot create %s: %v", dir, sub, err)
		return
	}
	dst := filepath.Join(dstDir, filepath.Base(full))
	if _, err := os.Stat(dst); err == nil {
		dst = filepath.Join(dstDir, fmt.Sprintf("%d-%s", p.Now().Unix(), filepath.Base(full)))
	}
	if err := os.Rename(full, dst); err != nil {
		p.Log.Printf("watch %s: cannot move %s: %v", dir, filepath.Base(full), err)
	}
}
