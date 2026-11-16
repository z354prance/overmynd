package arr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+c.prefix()+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Api-Key", c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s is not reachable", c.Kind)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{Code: resp.StatusCode, Kind: c.Kind}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// StatusError is a non-200 answer from an arr.
type StatusError struct {
	Code int
	Kind string
}

func (e *StatusError) Error() string { return fmt.Sprintf("%s answered HTTP %d", e.Kind, e.Code) }

// Grab is what the arr's history knows about a download: which series and episode.
type Grab struct{ SeriesID, EpisodeID int64 }

// Grabs lists the series and episodes a download ID was grabbed for.
func (c *Client) Grabs(ctx context.Context, downloadID string) ([]Grab, error) {
	q := url.Values{"page": {"1"}, "pageSize": {"100"}, "downloadId": {downloadID}}
	var r struct {
		Records []struct {
			SeriesID  int64 `json:"seriesId"`
			EpisodeID int64 `json:"episodeId"`
		} `json:"records"`
	}
	if err := c.get(ctx, "/history?"+q.Encode(), &r); err != nil {
		return nil, err
	}
	seen := map[Grab]bool{}
	var out []Grab
	for _, x := range r.Records {
		g := Grab{x.SeriesID, x.EpisodeID}
		if g.SeriesID != 0 && !seen[g] {
			seen[g] = true
			out = append(out, g)
		}
	}
	return out, nil
}

// SeriesInfo is the title and runtime (minutes) of a Sonarr series.
func (c *Client) SeriesInfo(ctx context.Context, id int64) (title string, runtime int, err error) {
	var s struct {
		Title   string `json:"title"`
		Runtime int    `json:"runtime"`
	}
	if err = c.get(ctx, fmt.Sprintf("/series/%d", id), &s); err != nil {
		return "", 0, err
	}
	return s.Title, s.Runtime, nil
}

// Episode is the part of a Sonarr episode the season counts need.
type Episode struct {
	ID         int64  `json:"id"`
	Season     int    `json:"seasonNumber"`
	Monitored  bool   `json:"monitored"`
	HasFile    bool   `json:"hasFile"`
	AirDateUTC string `json:"airDateUtc"`
}

func (c *Client) Episodes(ctx context.Context, seriesID int64) ([]Episode, error) {
	var eps []Episode
	err := c.get(ctx, fmt.Sprintf("/episode?seriesId=%d", seriesID), &eps)
	return eps, err
}

// Missing is one monitored item an arr wants and does not have.
type Missing struct {
	Kind   string     `json:"kind"` // episode | movie | album
	Title  string     `json:"title"`
	Detail string     `json:"detail,omitempty"` // S02E05, year, album name
	Date   *time.Time `json:"date,omitempty"`
}

func parseTime(s string) *time.Time {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	return nil
}

// MissingItems reads the arr's wanted/missing list (monitored items only).
func (c *Client) MissingItems(ctx context.Context) ([]Missing, error) {
	switch c.Kind {
	case "sonarr":
		var r struct {
			Records []struct {
				Title      string `json:"title"`
				Season     int    `json:"seasonNumber"`
				Episode    int    `json:"episodeNumber"`
				AirDateUTC string `json:"airDateUtc"`
				Monitored  bool   `json:"monitored"`
				Series     struct {
					Title string `json:"title"`
				} `json:"series"`
			} `json:"records"`
		}
		if err := c.get(ctx, "/wanted/missing?page=1&pageSize=1000&includeSeries=true&sortKey=airDateUtc&sortDirection=descending", &r); err != nil {
			return nil, err
		}
		var out []Missing
		for _, x := range r.Records {
			if !x.Monitored {
				continue
			}
			t := x.Series.Title
			if t == "" {
				t = x.Title
			}
			out = append(out, Missing{Kind: "episode", Title: t, Detail: fmt.Sprintf("S%02dE%02d", x.Season, x.Episode), Date: parseTime(x.AirDateUTC)})
		}
		return out, nil
	case "radarr":
		var r struct {
			Records []struct {
				Title          string `json:"title"`
				Year           int    `json:"year"`
				Monitored      bool   `json:"monitored"`
				DigitalRelease string `json:"digitalRelease"`
				InCinemas      string `json:"inCinemas"`
			} `json:"records"`
		}
		if err := c.get(ctx, "/wanted/missing?page=1&pageSize=1000&sortDirection=descending", &r); err != nil {
			return nil, err
		}
		var out []Missing
		for _, x := range r.Records {
			if !x.Monitored {
				continue
			}
			d := parseTime(x.DigitalRelease)
			if d == nil {
				d = parseTime(x.InCinemas)
			}
			out = append(out, Missing{Kind: "movie", Title: x.Title, Detail: fmt.Sprint(x.Year), Date: d})
		}
		return out, nil
	case "lidarr":
		var r struct {
			Records []struct {
				Title       string `json:"title"`
				Monitored   bool   `json:"monitored"`
				ReleaseDate string `json:"releaseDate"`
				Artist      struct {
					Name string `json:"artistName"`
				} `json:"artist"`
			} `json:"records"`
		}
		if err := c.get(ctx, "/wanted/missing?page=1&pageSize=1000&includeArtist=true&sortDirection=descending", &r); err != nil {
			return nil, err
		}
		var out []Missing
		for _, x := range r.Records {
			if !x.Monitored {
				continue
			}
			out = append(out, Missing{Kind: "album", Title: x.Artist.Name, Detail: x.Title, Date: parseTime(x.ReleaseDate)})
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s has no missing list", c.Kind)
}

// Import is one recent import event, named for display.
type Import struct {
	Kind   string    `json:"kind"` // episode | movie | album
	Title  string    `json:"title"`
	Date   time.Time `json:"date"`
	Detail string    `json:"detail,omitempty"` // "S01E02 - Episode title" for episodes
	Year   int       `json:"year,omitempty"`
	Poster string    `json:"poster,omitempty"` // /MediaCover/<id>/poster.jpg on the arr
	Arr    string    `json:"arr,omitempty"`    // category name, filled in by the caller
}

// RecentImports reads the newest import events from the arr's history.
func (c *Client) RecentImports(ctx context.Context) ([]Import, error) {
	var r struct {
		Records []struct {
			EventType string    `json:"eventType"`
			SeriesID  int64     `json:"seriesId"`
			MovieID   int64     `json:"movieId"`
			AlbumID   int64     `json:"albumId"`
			Date      time.Time `json:"date"`
			Series    struct {
				Title string `json:"title"`
			} `json:"series"`
			Episode struct {
				Title  string `json:"title"`
				Season int    `json:"seasonNumber"`
				Number int    `json:"episodeNumber"`
			} `json:"episode"`
			Movie struct {
				Title string `json:"title"`
				Year  int    `json:"year"`
			} `json:"movie"`
			Artist struct {
				Name string `json:"artistName"`
			} `json:"artist"`
			Album struct {
				Title string `json:"title"`
			} `json:"album"`
		} `json:"records"`
	}
	q := "/history?page=1&pageSize=100&sortKey=date&sortDirection=descending&includeSeries=true&includeEpisode=true&includeMovie=true&includeArtist=true&includeAlbum=true"
	if err := c.get(ctx, q, &r); err != nil {
		return nil, err
	}
	var out []Import
	for _, x := range r.Records {
		if !strings.Contains(strings.ToLower(x.EventType), "imported") {
			continue
		}
		switch c.Kind {
		case "sonarr":
			d := fmt.Sprintf("S%02dE%02d", x.Episode.Season, x.Episode.Number)
			if x.Episode.Title != "" {
				d += " - " + x.Episode.Title
			}
			out = append(out, Import{Kind: "episode", Title: x.Series.Title, Date: x.Date, Detail: d, Poster: cover(x.SeriesID)})
		case "radarr":
			out = append(out, Import{Kind: "movie", Title: x.Movie.Title, Date: x.Date, Year: x.Movie.Year, Poster: cover(x.MovieID)})
		case "lidarr":
			t := x.Artist.Name
			if x.Album.Title != "" {
				t += " — " + x.Album.Title
			}
			out = append(out, Import{Kind: "album", Title: t, Date: x.Date, Poster: albumCover(x.AlbumID)})
		}
	}
	return out, nil
}

func cover(id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("/MediaCover/%d/poster.jpg", id)
}

func albumCover(id int64) string {
	if id <= 0 {
		return ""
	}
	return fmt.Sprintf("/MediaCover/Albums/%d/cover.jpg", id)
}

// GrabbedResolution is the vertical resolution (480, 720, 1080, 2160) of the quality the
// arr grabbed a download as, or 0 when the arr does not say (music, unknown quality, no
// history for the download).
func (c *Client) GrabbedResolution(ctx context.Context, downloadID string) (int, error) {
	if c.Kind != "sonarr" && c.Kind != "radarr" {
		return 0, nil
	}
	q := url.Values{"page": {"1"}, "pageSize": {"100"}, "downloadId": {downloadID}}
	var r struct {
		Records []struct {
			EventType string `json:"eventType"`
			Quality   struct {
				Quality struct {
					Resolution int `json:"resolution"`
				} `json:"quality"`
			} `json:"quality"`
		} `json:"records"`
	}
	if err := c.get(ctx, "/history?"+q.Encode(), &r); err != nil {
		return 0, err
	}
	best := 0
	for _, x := range r.Records {
		if x.EventType == "grabbed" && x.Quality.Quality.Resolution > best {
			best = x.Quality.Quality.Resolution
		}
	}
	return best, nil
}
