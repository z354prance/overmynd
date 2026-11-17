// Package rss polls feeds and starts downloads for items that match a rule.
package rss

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/pipeline"
)

type Poller struct {
	Cfg    *config.Config
	Engine *pipeline.Engine
	Log    *log.Logger
	Now    func() time.Time
	HTTP   *http.Client

	mu     sync.Mutex
	seen   map[string]time.Time // item key -> when first seen
	last   map[string]time.Time // feed name -> last poll
	loaded bool
	path   string

	status  map[string]*FeedStatus // feed name -> what the last check saw
	outcome map[string]string      // item key -> what was done with it (this run only)
	polled  map[string][]item      // feed name -> items of the last check, in the order shown
}

// FeedStatus is what the last check of a feed saw, for the admin page.
type FeedStatus struct {
	Name      string     `json:"name"`
	CheckedAt time.Time  `json:"checkedAt"`
	Error     string     `json:"error,omitempty"`
	Items     []ItemView `json:"items"`
}

type ItemView struct {
	Title   string `json:"title"`
	Outcome string `json:"outcome"`
}

// Status returns the last check of every feed, in the order the feeds are configured.
func (p *Poller) Status() []FeedStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []FeedStatus{}
	for _, f := range p.Cfg.Get().Feeds {
		if st, ok := p.status[f.Name]; ok {
			out = append(out, *st)
		} else {
			out = append(out, FeedStatus{Name: f.Name, Items: []ItemView{}})
		}
	}
	return out
}

// CheckNow makes every feed due on the next tick and runs it.
func (p *Poller) CheckNow(ctx context.Context) {
	p.mu.Lock()
	p.last = map[string]time.Time{}
	p.mu.Unlock()
	p.Tick(ctx)
}

func New(c *config.Config, e *pipeline.Engine, l *log.Logger) *Poller {
	return &Poller{Cfg: c, Engine: e, Log: l, Now: time.Now, HTTP: &http.Client{Timeout: 60 * time.Second},
		seen: map[string]time.Time{}, last: map[string]time.Time{}, status: map[string]*FeedStatus{}, polled: map[string][]item{}, outcome: map[string]string{}, path: filepath.Join(c.Env.ConfigDir, "rss-seen.json")}
}

func (p *Poller) load() {
	if p.loaded {
		return
	}
	p.loaded = true
	if b, err := os.ReadFile(p.path); err == nil {
		_ = json.Unmarshal(b, &p.seen)
	}
}

func (p *Poller) save() {
	if len(p.seen) > 20000 { // keep the newest 15000
		type kv struct {
			k string
			t time.Time
		}
		var all []kv
		for k, t := range p.seen {
			all = append(all, kv{k, t})
		}
		cut := p.Now().Add(-90 * 24 * time.Hour)
		for _, x := range all {
			if x.t.Before(cut) {
				delete(p.seen, x.k)
			}
		}
	}
	b, _ := json.Marshal(p.seen)
	tmp := p.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, p.path)
	}
}

// Run polls until ctx ends.
func (p *Poller) Run(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		p.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick polls every feed that is due.
func (p *Poller) Tick(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.load()
	for _, f := range p.Cfg.Get().Feeds {
		if !f.Enabled || f.URL == "" || ctx.Err() != nil {
			continue
		}
		every := time.Duration(f.IntervalMinutes) * time.Minute
		if every < time.Minute {
			every = 15 * time.Minute
		}
		if last, ok := p.last[f.Name]; ok && p.Now().Sub(last) < every {
			continue
		}
		p.last[f.Name] = p.Now()
		if err := p.poll(ctx, f); err != nil {
			p.Log.Printf("rss %q: %v", f.Name, err)
			st := p.status[f.Name]
			if st == nil {
				st = &FeedStatus{Name: f.Name, Items: []ItemView{}}
				p.status[f.Name] = st
			}
			st.CheckedAt, st.Error = p.Now(), err.Error()
		}
	}
}

type item struct {
	Title     string `xml:"title"`
	GUID      string `xml:"guid"`
	Link      string `xml:"link"`
	Enclosure struct {
		URL string `xml:"url,attr"`
	} `xml:"enclosure"`
}

func (i item) key(feed string) string {
	k := i.GUID
	if k == "" {
		k = i.link()
	}
	return feed + "\x00" + k
}

func (i item) link() string {
	if i.Enclosure.URL != "" {
		return strings.TrimSpace(i.Enclosure.URL)
	}
	return strings.TrimSpace(i.Link)
}

func parse(b []byte) ([]item, error) {
	var doc struct {
		Items []item `xml:"channel>item"`
	}
	if err := xml.Unmarshal(b, &doc); err != nil {
		return nil, err
	}
	return doc.Items, nil
}

func (p *Poller) fetch(ctx context.Context, u string, max int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.HTTP.Do(req)
	if err != nil {
		return nil, errors.New("request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, max))
}

func (p *Poller) poll(ctx context.Context, f config.Feed) error {
	b, err := p.fetch(ctx, f.URL, 8<<20)
	if err != nil {
		return fmt.Errorf("fetch feed: %w", err)
	}
	items, err := parse(b)
	if err != nil {
		return fmt.Errorf("read feed: %w", err)
	}
	// The first poll of a feed only records what is already there, so adding a feed
	// or a rule never starts a flood of old downloads.
	known := false
	prefix := f.Name + "\x00"
	for k := range p.seen {
		if strings.HasPrefix(k, prefix) {
			known = true
			break
		}
	}
	// Feeds list newest first; start the oldest of the new items first.
	for i := len(items) - 1; i >= 0; i-- {
		it := items[i]
		k := it.key(f.Name)
		if _, ok := p.seen[k]; ok {
			continue
		}
		p.seen[k] = p.Now()
		if known {
			p.outcome[k] = p.apply(ctx, f, it)
		} else {
			p.outcome[k] = "recorded on the first check, not downloaded"
		}
	}
	st := &FeedStatus{Name: f.Name, CheckedAt: p.Now(), Items: []ItemView{}}
	for i, it := range items {
		if i >= 100 {
			break
		}
		o, ok := p.outcome[it.key(f.Name)]
		if !ok {
			o = "seen earlier"
		}
		st.Items = append(st.Items, ItemView{Title: it.Title, Outcome: o})
	}
	p.status[f.Name] = st
	p.polled[f.Name] = items[:min(len(items), 100)]
	if !known && len(items) == 0 {
		p.seen[prefix+"empty"] = p.Now()
	}
	p.save()
	return nil
}

func ci(pat string) (*regexp.Regexp, error) {
	if strings.TrimSpace(pat) == "" {
		return nil, nil
	}
	return regexp.Compile("(?i)" + pat)
}

// apply starts the item through the first enabled rule that matches its title.
func (p *Poller) apply(ctx context.Context, f config.Feed, it item) string {
	for _, r := range p.Cfg.Get().Rules {
		if !r.Enabled || (strings.TrimSpace(r.Feed) != "" && !strings.EqualFold(strings.TrimSpace(r.Feed), strings.TrimSpace(f.Name))) {
			continue
		}
		inc, err1 := ci(r.MustContain)
		exc, err2 := ci(r.MustNotContain)
		if err1 != nil || err2 != nil {
			continue
		}
		if inc != nil && !inc.MatchString(it.Title) {
			continue
		}
		if exc != nil && exc.MatchString(it.Title) {
			continue
		}
		if err := p.add(ctx, r, it); err != nil {
			p.Log.Printf("rss %q: rule %q matched %q but adding failed: %v", f.Name, r.Name, it.Title, err)
			return fmt.Sprintf("rule %q matched but adding failed: %v", r.Name, err)
		}
		p.Log.Printf("rss %q: rule %q started %q", f.Name, r.Name, it.Title)
		return fmt.Sprintf("started by rule %q", r.Name)
	}
	return "no rule matched"
}

func (p *Poller) add(ctx context.Context, r config.Rule, it item) error {
	link := it.link()
	switch {
	case strings.HasPrefix(link, "magnet:"):
		_, err := p.Engine.SubmitContext(ctx, pipeline.SubmitRequest{Source: "rss", Category: r.Category, Magnet: link})
		return err
	case strings.HasPrefix(link, "http://"), strings.HasPrefix(link, "https://"):
		data, err := p.fetch(ctx, link, 50<<20)
		if err != nil {
			return err
		}
		head := data[:min(len(data), 2048)]
		if bytes.Contains(head, []byte("<nzb")) {
			_, err = p.Engine.SubmitNZBContext(ctx, pipeline.SubmitNZBRequest{Source: "rss", Category: r.Category, Name: it.Title, NZB: data})
			return err
		}
		if len(data) > 0 && data[0] == 'd' {
			_, err = p.Engine.SubmitContext(ctx, pipeline.SubmitRequest{Source: "rss", Category: r.Category, Torrent: data})
			return err
		}
		return errors.New("the link is neither a torrent nor an NZB")
	}
	return errors.New("item has no usable link")
}

// Download starts one item of a feed by hand, in the chosen category, whether or not
// any rule matched it. index and title name the item as the admin page last showed it,
// so a feed that changed since is refused rather than starting the wrong release.
func (p *Poller) Download(ctx context.Context, feed string, index int, title, category string) (string, error) {
	if _, ok := p.Cfg.Get().Category(category); !ok {
		return "", errors.New("choose one of your categories")
	}
	p.mu.Lock()
	items := p.polled[feed]
	if index < 0 || index >= len(items) || items[index].Title != title {
		p.mu.Unlock()
		return "", errors.New("the feed changed since it was last checked; check feeds now and try again")
	}
	it := items[index]
	p.mu.Unlock()
	if err := p.add(ctx, config.Rule{Category: category}, it); err != nil {
		return "", err
	}
	outcome := "started by hand in " + category
	p.mu.Lock()
	p.outcome[it.key(feed)] = outcome
	if st := p.status[feed]; st != nil && index < len(st.Items) && st.Items[index].Title == title {
		st.Items[index].Outcome = outcome
	}
	p.mu.Unlock()
	p.Log.Printf("rss %q: %q started by hand in %s", feed, title, category)
	return outcome, nil
}
