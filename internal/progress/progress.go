// Package progress works out, for the public page, how far each show in the
// pipeline is through its seasons, and what the arrs still want and do not have.
package progress

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/z354prance/overmynd/internal/arr"
	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/store"
)

type Season struct {
	ShowID   string `json:"showId"`
	Show     string `json:"show"`
	Season   int    `json:"season"`
	Imported int    `json:"imported"`
	Total    int    `json:"total"`
}

// Recent is a run of imports of one title on one day: "3 episodes of X".
type Recent struct {
	Kind     string    `json:"kind"`
	Title    string    `json:"title"`
	Count    int       `json:"count"`
	Date     time.Time `json:"date"`
	Year     int       `json:"year,omitempty"`
	Episodes []string  `json:"episodes,omitempty"`
	Poster   string    `json:"poster,omitempty"`
	Arr      string    `json:"arr,omitempty"`
}

type Snapshot struct {
	Recent    []Recent  `json:"recent"`
	Seasons   []Season  `json:"seasons"`
	Missing   []Missing `json:"missing"`
	Errors    []string  `json:"errors"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Missing struct {
	Category string `json:"category"`
	arr.Missing
}

type tracked struct {
	Category string    `json:"category"`
	SeriesID int64     `json:"seriesId"`
	Since    time.Time `json:"since"`
}

type Tracker struct {
	Cfg   *config.Config
	Store *store.Store
	Log   *log.Logger
	Now   func() time.Time
	// ArrFor builds the client for a category; tests replace it.
	ArrFor func(config.Category) *arr.Client

	mu       sync.Mutex
	path     string
	tracked  map[string]tracked    // "category:series" -> entry
	grabs    map[string][]arr.Grab // job ID -> what history said
	snap     Snapshot
	missAt   time.Time
	missing  []Missing
	missErrs []string
	recent   []Recent
}

func New(c *config.Config, s *store.Store, l *log.Logger) *Tracker {
	t := &Tracker{Cfg: c, Store: s, Log: l, Now: time.Now, path: filepath.Join(c.Env.ConfigDir, "tracked-shows.json"),
		tracked: map[string]tracked{}, grabs: map[string][]arr.Grab{}, snap: Snapshot{Seasons: []Season{}, Missing: []Missing{}, Errors: []string{}}}
	t.ArrFor = func(cat config.Category) *arr.Client { return arr.New(cat.Kind, cat.ArrURL, string(cat.ArrAPIKey)) }
	if b, err := os.ReadFile(t.path); err == nil {
		_ = json.Unmarshal(b, &t.tracked)
	}
	return t
}

func (t *Tracker) save() {
	b, _ := json.Marshal(t.tracked)
	tmp := t.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		_ = os.Rename(tmp, t.path)
	}
}

// Snapshot is the last computed view.
func (t *Tracker) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snap
}

// Run refreshes once a minute until ctx ends.
func (t *Tracker) Run(ctx context.Context) {
	tk := time.NewTicker(time.Minute)
	defer tk.Stop()
	for {
		t.Refresh(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tk.C:
		}
	}
}

func usable(c config.Category) bool {
	return c.ArrURL != "" && c.ArrAPIKey != "" && (c.Kind == "sonarr" || c.Kind == "radarr" || c.Kind == "lidarr")
}

// Refresh recomputes the snapshot.
func (t *Tracker) Refresh(ctx context.Context) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.Now()
	s := t.Cfg.Get()
	var errs []string

	// 1. Learn which series and episodes the active Sonarr jobs are for.
	queued := map[string]map[int64]bool{}
	live := map[string]bool{}
	for _, j := range t.Store.List() {
		if j.State == job.Cleaned || j.State == job.Failed || j.Source == "rss" || j.Source == "watch" {
			continue
		}
		cat, ok := s.Category(j.Category)
		if !ok || cat.Kind != "sonarr" || !usable(cat) {
			continue
		}
		live[j.ID] = true
		if _, known := t.grabs[j.ID]; !known {
			g, err := t.ArrFor(cat).Grabs(ctx, j.ID)
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", cat.Name, err))
				continue
			}
			t.grabs[j.ID] = g
		}
		for _, g := range t.grabs[j.ID] {
			key := cat.Name + ":" + strconv.FormatInt(g.SeriesID, 10)
			if queued[key] == nil {
				queued[key] = map[int64]bool{}
			}
			if g.EpisodeID != 0 {
				queued[key][g.EpisodeID] = true
			}
			if _, ok := t.tracked[key]; !ok {
				t.tracked[key] = tracked{Category: cat.Name, SeriesID: g.SeriesID, Since: now}
			}
		}
	}
	for id := range t.grabs {
		if !live[id] {
			delete(t.grabs, id)
		}
	}

	// 2. Season counts for every tracked series.
	seasons := []Season{}
	for key, tr := range t.tracked {
		cat, ok := s.Category(tr.Category)
		if !ok || !usable(cat) {
			delete(t.tracked, key)
			continue
		}
		c := t.ArrFor(cat)
		title, runtime, err := c.SeriesInfo(ctx, tr.SeriesID)
		if err != nil {
			var se *arr.StatusError
			if errors.As(err, &se) && se.Code == http.StatusNotFound && now.Sub(tr.Since) > 7*24*time.Hour {
				delete(t.tracked, key) // removed from Sonarr and long idle
			} else {
				errs = append(errs, fmt.Sprintf("%s series %d: %v", cat.Name, tr.SeriesID, err))
			}
			continue
		}
		eps, err := c.Episodes(ctx, tr.SeriesID)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s series %d: %v", cat.Name, tr.SeriesID, err))
			continue
		}
		q := queued[key]
		if len(q) == 0 {
			q = map[int64]bool{0: true} // tracked earlier; keep it active until complete
		}
		counts := CountSeasons(eps, q, now, runtime)
		complete := len(eps) > 0
		for _, n := range counts {
			if n.Imported < n.Total {
				complete = false
			}
		}
		if complete {
			delete(t.tracked, key)
			continue
		}
		for _, n := range counts {
			n.Show = title
			n.ShowID = key
			seasons = append(seasons, n)
		}
	}
	sort.Slice(seasons, func(i, j int) bool {
		if seasons[i].Show != seasons[j].Show {
			return strings.ToLower(seasons[i].Show) < strings.ToLower(seasons[j].Show)
		}
		return seasons[i].Season < seasons[j].Season
	})
	t.save()

	// 3. Missing items, every five minutes.
	if now.Sub(t.missAt) >= 5*time.Minute {
		t.missAt = now
		var all []Missing
		t.missErrs = nil
		var imports []arr.Import
		for _, cat := range s.Categories {
			if !usable(cat) {
				continue
			}
			if ims, ierr := t.ArrFor(cat).RecentImports(ctx); ierr != nil {
				t.missErrs = append(t.missErrs, fmt.Sprintf("%s history: %v", cat.Name, ierr))
			} else {
				for i := range ims {
					ims[i].Arr = cat.Name
				}
				imports = append(imports, ims...)
			}
			items, err := t.ArrFor(cat).MissingItems(ctx)
			if err != nil {
				t.missErrs = append(t.missErrs, fmt.Sprintf("%s: %v", cat.Name, err))
				continue
			}
			for _, m := range items {
				all = append(all, Missing{Category: cat.Name, Missing: m})
			}
		}
		sort.SliceStable(all, func(i, j int) bool {
			a, b := all[i].Date, all[j].Date
			if a == nil || b == nil {
				return a != nil
			}
			return a.After(*b)
		})
		if len(all) > 1000 {
			all = all[:1000]
		}
		t.missing = all
		t.recent = groupRecent(imports, 12)
	}
	errs = append(errs, t.missErrs...)
	if t.missing == nil {
		t.missing = []Missing{}
	}
	if errs == nil {
		errs = []string{}
	}
	if t.recent == nil {
		t.recent = []Recent{}
	}
	t.snap = Snapshot{Recent: t.recent, Seasons: seasons, Missing: t.missing, Errors: errs, UpdatedAt: now}
}

// CountSeasons counts monitored episodes per season of a series that is active:
// those with a file, plus aired ones without. Episodes not yet aired (or whose
// broadcast has not ended) and undated ones are not missing; a queued episode counts.
func CountSeasons(eps []arr.Episode, queued map[int64]bool, now time.Time, runtime int) map[int]Season {
	counts := map[int]Season{}
	for _, ep := range eps {
		if !ep.Monitored {
			continue
		}
		if !ep.HasFile && !queued[ep.ID] {
			aired, err := time.Parse(time.RFC3339, ep.AirDateUTC)
			if err != nil || aired.Add(time.Duration(max(runtime, 0))*time.Minute).After(now) {
				continue
			}
		}
		c := counts[ep.Season]
		c.Season = ep.Season
		c.Total++
		if ep.HasFile {
			c.Imported++
		}
		counts[ep.Season] = c
	}
	return counts
}

// groupRecent folds imports of the same title on the same day into one entry,
// newest first, keeping at most n.
func groupRecent(in []arr.Import, n int) []Recent {
	type key struct {
		kind, title, day string
	}
	idx := map[key]int{}
	var out []Recent
	for _, im := range in {
		k := key{im.Kind, im.Title, im.Date.Local().Format("2006-01-02")}
		if i, ok := idx[k]; ok {
			out[i].Count++
			if im.Detail != "" {
				out[i].Episodes = append(out[i].Episodes, im.Detail)
			}
			if im.Date.After(out[i].Date) {
				out[i].Date = im.Date
			}
			continue
		}
		idx[k] = len(out)
		r := Recent{Kind: im.Kind, Title: im.Title, Count: 1, Date: im.Date, Year: im.Year, Poster: im.Poster, Arr: im.Arr}
		if im.Detail != "" {
			r.Episodes = []string{im.Detail}
		}
		out = append(out, r)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}
