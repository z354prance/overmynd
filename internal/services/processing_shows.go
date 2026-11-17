package services

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/z354prance/overmynd/internal/integrations"
	"github.com/z354prance/overmynd/internal/integrations/arr"
	"github.com/z354prance/overmynd/internal/models"
)

var processingEpisodeMarker = regexp.MustCompile(`(?i)[ ._-]+s[0-9]{1,3}e[0-9]{1,3}(?:[^0-9]|$)`)

func showNameKey(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, name)
}

// Require episode coordinates and an exact normalized series title. Never use
// a substring match: remakes and similarly named shows must remain separate.
func processingShowName(job models.ProcessingJob) string {
	title := job.Title
	if title == "" {
		title = path.Base(strings.ReplaceAll(job.Path, `\`, "/"))
	}
	match := processingEpisodeMarker.FindStringIndex(title)
	if match == nil {
		return ""
	}
	return showNameKey(title[:match[0]])
}

type processingSeries struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Year  int    `json:"year"`
}

func (m *Manager) processingShows(ctx context.Context, jobs []models.ProcessingJob) ([]models.Download, []ServiceError) {
	names := map[string]bool{}
	for _, job := range jobs {
		if job.Source != models.ServiceTdarr {
			continue
		}
		switch job.State {
		case models.ProcessingStateProcessing, models.ProcessingStateProblem:
			if name := processingShowName(job); name != "" {
				names[name] = true
			}
		}
	}
	matches := []models.Download{}
	errors := []ServiceError{}
	if len(names) == 0 {
		return matches, errors
	}
	services, err := m.List()
	if err != nil {
		return matches, errors
	}
	for _, service := range services {
		if !service.Enabled || service.Type != models.ServiceSonarr {
			continue
		}
		credential, err := m.Credential(service.ID)
		if err != nil {
			errors = append(errors, serviceError(service, err))
			continue
		}
		endpoint, err := arr.NewClient("v3").Endpoint(service.BaseURL, "series")
		if err != nil {
			errors = append(errors, serviceError(service, err))
			continue
		}
		var series []processingSeries
		if err := integrations.NewHTTPClient().GetJSON(ctx, endpoint, map[string]string{"X-Api-Key": credential}, &series); err != nil {
			errors = append(errors, serviceError(service, fmt.Errorf("unable to identify processing series in Sonarr")))
			continue
		}
		candidates := map[string]map[int64]bool{}
		for _, show := range series {
			keys := []string{showNameKey(show.Title)}
			if show.Year > 0 {
				keys = append(keys, showNameKey(show.Title+strconv.Itoa(show.Year)))
			}
			for _, key := range keys {
				if candidates[key] == nil {
					candidates[key] = map[int64]bool{}
				}
				candidates[key][show.ID] = true
			}
		}
		matched := map[int64]bool{}
		for name := range names {
			if len(candidates[name]) != 1 {
				continue
			}
			for id := range candidates[name] {
				matched[id] = true
			}
		}
		for id := range matched {
			matches = append(matches, models.Download{Source: models.ServiceSonarr, SourceServiceID: service.ID, SeriesID: id})
		}
	}
	return matches, errors
}
