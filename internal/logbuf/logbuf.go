// Package logbuf keeps the most recent log lines in memory so the admin page can show them.
package logbuf

import (
	"strings"
	"sync"
)

// Buffer is an io.Writer that remembers the last Max lines.
type Buffer struct {
	Max int

	mu    sync.Mutex
	lines []string
	part  string
}

// New returns a Buffer that keeps the last max lines.
func New(max int) *Buffer { return &Buffer{Max: max} }

// Write splits what it is given into lines. The one-time admin setup key is never kept.
func (b *Buffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := b.part + string(p)
	parts := strings.Split(s, "\n")
	b.part = parts[len(parts)-1]
	for _, l := range parts[:len(parts)-1] {
		if l == "" || strings.Contains(strings.ToLower(l), "setup key") {
			continue
		}
		b.lines = append(b.lines, l)
	}
	if over := len(b.lines) - b.Max; over > 0 {
		b.lines = append([]string(nil), b.lines[over:]...)
	}
	return len(p), nil
}

// Lines returns up to n of the newest lines (oldest first) that contain every word of
// filter, ignoring case. An empty filter keeps everything.
func (b *Buffer) Lines(filter string, n int) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	words := strings.Fields(strings.ToLower(filter))
	var out []string
	for i := len(b.lines) - 1; i >= 0 && len(out) < n; i-- {
		l := strings.ToLower(b.lines[i])
		ok := true
		for _, w := range words {
			if !strings.Contains(l, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, b.lines[i])
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
