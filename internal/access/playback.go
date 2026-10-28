package access

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/z354prance/overmynd/internal/models"
)

// FilterPlayback uses the configured Emby server to resolve missing library IDs.
// When enabled, unidentified Emby sessions are withheld rather than made public.
// Deployments with multiple Emby servers must not use this single-server filter.
func (m *Manager) FilterPlayback(ctx context.Context, sessions []models.PlaybackSession, excluded string) []models.PlaybackSession {
	blocked := map[string]bool{}
	for _, id := range strings.Split(excluded, ",") {
		if id = strings.TrimSpace(id); id != "" {
			blocked[id] = true
		}
	}
	if len(blocked) == 0 {
		return sessions
	}
	settings, err := m.Settings()
	if err != nil {
		settings = Settings{}
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return filterPlayback(ctx, settings, sessions, blocked)
}

type playbackLibrary struct {
	ID     string `json:"Id"`
	ItemID string `json:"ItemId"`
}

func filterPlayback(ctx context.Context, settings Settings, sessions []models.PlaybackSession, blocked map[string]bool) []models.PlaybackSession {
	result := make([]models.PlaybackSession, 0, len(sessions))
	// Fetch library roots only if a session lacks its library identity.
	var roots map[string]bool
	loadedRoots := false
	for _, session := range sessions {
		if !strings.EqualFold(session.ServerType, "emby") {
			// Missing server identity cannot establish that this is outside the filter.
			if session.ServerType != "" {
				result = append(result, session)
			}
			continue
		}
		if session.LibraryID != "" {
			if !blocked[session.LibraryID] {
				result = append(result, session)
			}
			continue
		}
		if settings.EmbyURL == "" || settings.EmbyKey == "" || !idPattern.MatchString(session.RatingKey) {
			continue
		}
		if !loadedRoots {
			loadedRoots = true
			var libraries []playbackLibrary
			if emby(ctx, settings, http.MethodGet, "/Library/VirtualFolders", nil, &libraries) == nil {
				roots = map[string]bool{}
				for _, library := range libraries {
					if library.ItemID != "" {
						roots[library.ItemID] = true
					}
				}
			}
		}
		if len(roots) == 0 {
			continue
		}
		var ancestors []playbackLibrary
		if emby(ctx, settings, http.MethodGet, "/Items/"+session.RatingKey+"/Ancestors", nil, &ancestors) != nil {
			continue
		}
		identified, hidden := false, false
		for _, ancestor := range ancestors {
			if blocked[ancestor.ID] {
				hidden = true
			}
			if roots[ancestor.ID] {
				identified = true
			}
		}
		if identified && !hidden {
			result = append(result, session)
		}
	}
	return result
}
