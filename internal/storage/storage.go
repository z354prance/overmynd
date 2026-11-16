// Package storage exposes cached, read-only library sizes without file names.
package storage

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Category struct {
	Name  string `json:"name"`
	Bytes *int64 `json:"bytes"`
}
type Snapshot struct {
	Enabled    bool       `json:"enabled"`
	Scanning   bool       `json:"scanning"`
	Categories []Category `json:"categories"`
	TotalBytes *uint64    `json:"total_bytes"`
	FreeBytes  *uint64    `json:"free_bytes"`
	UpdatedAt  time.Time  `json:"updated_at"`
}
type Monitor struct {
	mu    sync.Mutex
	root  string
	path  string // where the last scan is saved; empty means memory only
	value Snapshot
}

func New(root string) *Monitor { return &Monitor{root: root, value: Snapshot{Enabled: root != ""}} }

// Persist saves every finished scan to path and loads the last saved scan now,
// so a restart shows the previous sizes straight away instead of an empty bar.
func (m *Monitor) Persist(path string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.path = path
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var saved Snapshot
	if json.Unmarshal(b, &saved) != nil || saved.UpdatedAt.IsZero() {
		return
	}
	saved.Enabled, saved.Scanning = m.root != "", false
	m.value = saved
}

// Snapshot returns the last scan. Readers never walk the disk and never start a scan.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.value
	v.Categories = append([]Category(nil), v.Categories...)
	return v
}

// Run scans in the background: at once when there is no saved scan or it is older than
// the interval, then once every interval. The interval is read each time so a settings
// change takes effect without a restart.
func (m *Monitor) Run(ctx context.Context, interval func() time.Duration) {
	if m.root == "" {
		return
	}
	for {
		every := interval()
		if every < time.Minute {
			every = time.Minute
		}
		m.mu.Lock()
		last := m.value.UpdatedAt
		m.mu.Unlock()
		wait := every - time.Since(last)
		if last.IsZero() || wait <= 0 {
			m.scanNow()
			wait = every
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

func (m *Monitor) scanNow() {
	m.mu.Lock()
	m.value.Scanning = true
	m.mu.Unlock()
	result := scan(m.root)
	m.mu.Lock()
	m.value = result
	path := m.path
	m.mu.Unlock()
	if path == "" {
		return
	}
	if b, err := json.Marshal(result); err == nil {
		tmp := path + ".tmp"
		if os.WriteFile(tmp, b, 0o600) == nil {
			_ = os.Rename(tmp, path)
		}
	}
}
func scan(root string) Snapshot {
	result := Snapshot{Enabled: true, UpdatedAt: time.Now().UTC()}
	for _, category := range []struct{ name, folder string }{{"Movies", "Movies"}, {"TV", "Episodes"}, {"Anime", "Anime"}, {"Music", "Music"}, {"Books", "Books"}} {
		value := Category{Name: category.name}
		if bytes, err := directorySize(filepath.Join(root, category.folder)); err == nil {
			value.Bytes = &bytes
		}
		result.Categories = append(result.Categories, value)
	}
	// Free capacity is that of the mounted media filesystem, not the container.
	if free, total, err := available(root); err == nil {
		result.TotalBytes = &total
		result.FreeBytes = &free
	}
	return result
}
func directorySize(root string) (int64, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return 0, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return 0, fs.ErrInvalid
	}
	var total int64
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 || entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}
