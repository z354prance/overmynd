package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// Limiter keeps every TorBox call inside its limits. Its counters are saved to
// disk so a restart does not reset the budget. It never fails a call: when a
// call cannot go out now it returns ErrBusy so the caller waits.
type Limiter struct {
	mu           sync.Mutex
	path         string
	Now          func() time.Time
	CreatePerHr  int           // budget for create calls
	LinkGap      time.Duration // minimum gap between download-link requests
	APIGap       time.Duration // minimum gap between other API calls
	creates      []time.Time
	backoffUntil time.Time
	lastLink     time.Time
	lastAPI      time.Time
}

type limiterState struct {
	Creates      []time.Time `json:"creates"`
	BackoffUntil time.Time   `json:"backoffUntil"`
}

func NewLimiter(path string, createPerHour int, linkGap time.Duration) *Limiter {
	l := &Limiter{path: path, Now: time.Now, CreatePerHr: createPerHour, LinkGap: linkGap, APIGap: 300 * time.Millisecond}
	if b, err := os.ReadFile(path); err == nil {
		var st limiterState
		if json.Unmarshal(b, &st) == nil {
			l.creates, l.backoffUntil = st.Creates, st.BackoffUntil
		}
	}
	return l
}

func (l *Limiter) saveLocked() {
	if l.path == "" {
		return
	}
	b, _ := json.Marshal(limiterState{Creates: l.creates, BackoffUntil: l.backoffUntil})
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, l.path)
	}
}

type Class int

const (
	API Class = iota
	Create
	Link
)

// Acquire reserves a slot for one call of the given class. A short pacing wait is
// done here; a limit or backoff returns an error wrapping ErrBusy.
func (l *Limiter) Acquire(ctx context.Context, c Class) error {
	l.mu.Lock()
	now := l.Now()
	if now.Before(l.backoffUntil) {
		d := l.backoffUntil.Sub(now)
		l.mu.Unlock()
		return fmt.Errorf("%w: TorBox asked us to wait %s", ErrBusy, d.Round(time.Second))
	}
	var wait time.Duration
	switch c {
	case Create:
		cut := now.Add(-time.Hour)
		kept := l.creates[:0]
		for _, t := range l.creates {
			if t.After(cut) {
				kept = append(kept, t)
			}
		}
		l.creates = kept
		if l.CreatePerHr > 0 && len(l.creates) >= l.CreatePerHr {
			d := l.creates[0].Add(time.Hour).Sub(now)
			l.mu.Unlock()
			return fmt.Errorf("%w: %d creates used this hour, next slot in %s", ErrBusy, l.CreatePerHr, d.Round(time.Second))
		}
		l.creates = append(l.creates, now)
		l.saveLocked()
	case Link:
		if next := l.lastLink.Add(l.LinkGap); next.After(now) {
			wait = next.Sub(now)
		}
		l.lastLink = now.Add(wait)
	default:
		if next := l.lastAPI.Add(l.APIGap); next.After(now) {
			wait = next.Sub(now)
		}
		l.lastAPI = now.Add(wait)
	}
	l.mu.Unlock()
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Backoff records a server-requested wait (HTTP 429 or Retry-After).
func (l *Limiter) Backoff(d time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if until := l.Now().Add(d); until.After(l.backoffUntil) {
		l.backoffUntil = until
		l.saveLocked()
	}
}

// Refund returns a create slot when the call never reached TorBox.
func (l *Limiter) Refund() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if n := len(l.creates); n > 0 {
		l.creates = l.creates[:n-1]
		l.saveLocked()
	}
}

// Used reports how many create calls are counted in the last hour.
func (l *Limiter) Used() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := l.Now().Add(-time.Hour)
	n := 0
	for _, t := range l.creates {
		if t.After(cut) {
			n++
		}
	}
	return n
}

// Status reports the create budget, for the admin System view.
func (l *Limiter) Status() (used, limit int, backoffUntil time.Time) {
	used = l.Used()
	l.mu.Lock()
	defer l.mu.Unlock()
	return used, l.CreatePerHr, l.backoffUntil
}
