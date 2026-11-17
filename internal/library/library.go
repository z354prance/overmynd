// Package library places finished release files into an existing show library
// laid out as <root>/<Show>/Season N/<Show> - <ep> (<res>).ext. It never guesses:
// anything it is not sure of is left for a person to decide.
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Release is what could be read from a release file name.
type Release struct {
	Title    string // the show title, without group, season marker, episode or tags
	Season   int    // 1 when the name carries no season marker
	Marked   bool   // the name said which season
	Episode  int
	EpisodeS string // the episode as written, e.g. "07"
	Res      string // e.g. "1080p", empty if none
}

var (
	lead    = regexp.MustCompile(`^\s*(\[[^\]]*\]\s*)+`)
	tail    = regexp.MustCompile(`(\s*\[[^\]]*\])+\s*$`)
	res     = regexp.MustCompile(`(?i)[\(\[ ](\d{3,4}p)[\)\] ]?`)
	epDash  = regexp.MustCompile(`(?i)^(.*?)\s+-\s+(?:EP?\.?\s?)?(\d{1,4})(?:v\d+)?(?:\s|\(|\[|$)`)
	epNxM   = regexp.MustCompile(`(?i)^(.*?)\s+(?:-\s+)?(\d{1,2})x(\d{1,3})(?:v\d+)?(?:\s|\(|\[|$)`)
	year    = regexp.MustCompile(`\s*[\(\[](?:19|20)\d\d[\)\]]\s*$`)
	epSxE   = regexp.MustCompile(`(?i)^(.*?)\s+(?:-\s+)?S(\d{1,2})E(\d{1,3})(?:v\d+)?(?:\s|\(|\[|\.|$)`)
	seasonA = regexp.MustCompile(`(?i)^(.*?)\s+S(\d{1,2})$`)
	seasonB = regexp.MustCompile(`(?i)^(.*?)\s+(\d{1,2})(?:st|nd|rd|th)\s+Season$`)
	seasonC = regexp.MustCompile(`(?i)^(.*?)\s+Season\s+(\d{1,2})$`)
)

// Parse reads a release file name (with or without its extension).
func Parse(name string) (Release, bool) {
	s := strings.TrimSuffix(name, filepath.Ext(name))
	if strings.EqualFold(filepath.Ext(name), "") {
		s = name
	}
	s = lead.ReplaceAllString(s, "")
	s = tail.ReplaceAllString(s, "")
	// Dotted or underscored names ("Show.Name.S02E06.1080p.WEB-GRP") read like spaced ones.
	if strings.Count(s, ".")+strings.Count(s, "_") > strings.Count(s, " ") {
		s = strings.Join(strings.Fields(strings.NewReplacer(".", " ", "_", " ").Replace(s)), " ")
	}
	r := Release{Season: 1}
	if m := res.FindStringSubmatch(" " + s + " "); m != nil {
		r.Res = strings.ToLower(m[1])
	}
	var title string
	if m := epSxE.FindStringSubmatch(s); m != nil {
		title = m[1]
		r.Season, _ = strconv.Atoi(m[2])
		r.Marked = true
		r.Episode, _ = strconv.Atoi(m[3])
		r.EpisodeS = m[3]
	} else if m := epNxM.FindStringSubmatch(s); m != nil {
		title = m[1]
		r.Season, _ = strconv.Atoi(m[2])
		r.Marked = true
		r.Episode, _ = strconv.Atoi(m[3])
		r.EpisodeS = m[3]
	} else if m := epDash.FindStringSubmatch(s); m != nil {
		title = m[1]
		r.Episode, _ = strconv.Atoi(m[2])
		r.EpisodeS = m[2]
	} else {
		return Release{}, false
	}
	title = strings.TrimSpace(title)
	title = strings.TrimSpace(year.ReplaceAllString(title, ""))
	for _, re := range []*regexp.Regexp{seasonA, seasonB, seasonC} {
		if m := re.FindStringSubmatch(title); m != nil {
			if n, err := strconv.Atoi(m[2]); err == nil && n > 0 {
				r.Season, r.Marked = n, true
				title = strings.TrimSpace(m[1])
				break
			}
		}
	}
	if title == "" {
		return Release{}, false
	}
	r.Title = title
	return r, true
}

// Normalize makes titles comparable: lower case, letters and digits only.
func Normalize(s string) string {
	var b strings.Builder
	space := true
	s = strings.NewReplacer("&", " and ", "'", "", "\u2019", "").Replace(s)
	for _, c := range strings.ToLower(s) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c > 127 {
			b.WriteRune(c)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}

// Folders lists the show folders under root.
func Folders(root string) ([]string, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// Match returns the show folder for a title when exactly one folder fits it
// exactly (ignoring case and punctuation), and otherwise the closest candidates.
func Match(title string, folders []string) (folder string, candidates []string) {
	want := Normalize(title)
	var exact []string
	for _, f := range folders {
		if Normalize(f) == want {
			exact = append(exact, f)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) == 0 {
		// "Show (2020)" and "Show" are the same show.
		bare := Normalize(year.ReplaceAllString(title, ""))
		for _, f := range folders {
			if Normalize(year.ReplaceAllString(f, "")) == bare {
				exact = append(exact, f)
			}
		}
		if len(exact) == 1 {
			return exact[0], nil
		}
	}
	type scored struct {
		name  string
		score float64
	}
	var sc []scored
	wt := tokens(want)
	for _, f := range folders {
		if s := jaccard(wt, tokens(Normalize(f))); s >= 0.4 {
			sc = append(sc, scored{f, s})
		}
	}
	sort.Slice(sc, func(i, j int) bool { return sc[i].score > sc[j].score })
	for i := 0; i < len(sc) && i < 3; i++ {
		candidates = append(candidates, sc[i].name)
	}
	return "", candidates
}

func tokens(s string) map[string]bool {
	m := map[string]bool{}
	for _, t := range strings.Fields(s) {
		m[t] = true
	}
	return m
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	in := 0
	for k := range a {
		if b[k] {
			in++
		}
	}
	return float64(in) / float64(len(a)+len(b)-in)
}

// SeasonDir is the folder for a season inside a show folder.
func SeasonDir(showDir string, season int) string {
	return filepath.Join(showDir, fmt.Sprintf("Season %d", season))
}

var (
	cleanLead = regexp.MustCompile(`^\[[^\]]+\]\s*`)
	cleanPad  = regexp.MustCompile(` - 0+(\d)`)
	cleanTail = regexp.MustCompile(`\s+\[.*(\.[^.]+)$`)
)

// Clean gives a release file name the library's name: the same three steps as the
// Rename.ps1 script used by hand. It drops the leading [Group], drops zero padding
// after " - ", and drops a trailing [hash] before the extension.
func Clean(name string) string {
	n := cleanLead.ReplaceAllString(name, "")
	n = cleanPad.ReplaceAllString(n, " - $1")
	n = cleanTail.ReplaceAllString(n, "$1")
	return n
}
