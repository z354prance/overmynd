// Package storage exposes cached, read-only library sizes without file names.
package storage

import (
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
	last  time.Time
	value Snapshot
}

func New(root string) *Monitor { return &Monitor{root: root, value: Snapshot{Enabled: root != ""}} }

// Snapshot starts at most one scan per 30 minutes; readers never walk the disk.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.root != "" && !m.value.Scanning && (m.last.IsZero() || time.Since(m.last) >= 30*time.Minute) {
		m.value.Scanning = true
		go func() {
			result := scan(m.root)
			m.mu.Lock()
			m.value = result
			m.last = time.Now()
			m.mu.Unlock()
		}()
	}
	return m.value
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
