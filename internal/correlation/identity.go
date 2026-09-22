package correlation

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/z354prance/overmynd/internal/models"
)

// Identity contains normalized correlation keys extracted from a source record.
type Identity struct {
	Kind models.MediaKind

	TMDBID        int64
	TVDBID        int64
	IMDbID        string
	MusicBrainzID string

	Source          models.ServiceType
	SourceServiceID int64

	MovieID   int64
	SeriesID  int64
	EpisodeID int64
	ArtistID  int64
	AlbumID   int64

	SeasonNumber  int
	EpisodeNumber int
}

// Match describes the strongest evidence shared by two identities.
type Match struct {
	Matched  bool
	Reason   models.CorrelationReason
	Strength models.CorrelationStrength
	Key      string
}

func IdentityFromAttention(item models.AttentionItem) Identity {
	identity := Identity{
		Kind:            item.Kind,
		TMDBID:          item.TMDBID,
		TVDBID:          item.TVDBID,
		IMDbID:          normalizeTextID(item.IMDbID),
		MusicBrainzID:   normalizeTextID(item.MusicBrainzID),
		Source:          item.Source,
		SourceServiceID: item.SourceServiceID,
		SeasonNumber:    item.SeasonNumber,
		EpisodeNumber:   item.EpisodeNumber,
	}

	switch item.Source {
	case models.ServiceRadarr:
		identity.MovieID = item.SourceID
	case models.ServiceSonarr:
		identity.EpisodeID = item.SourceID
	case models.ServiceLidarr:
		identity.AlbumID = item.SourceID
	}

	return identity
}

func IdentityFromRequest(item models.Request) Identity {
	return Identity{
		Kind:            mediaKindFromRequest(item.MediaType),
		TMDBID:          item.TMDBID,
		Source:          item.Source,
		SourceServiceID: item.SourceServiceID,
	}
}

func IdentityFromDownload(item models.Download) Identity {
	return Identity{
		Source:          item.Source,
		SourceServiceID: item.SourceServiceID,
		MovieID:         item.MovieID,
		SeriesID:        item.SeriesID,
		EpisodeID:       item.EpisodeID,
		ArtistID:        item.ArtistID,
		AlbumID:         item.AlbumID,
	}
}

func IdentityFromPlayback(item models.PlaybackSession) Identity {
	return Identity{
		Kind:            mediaKindFromPlayback(item.MediaType),
		TMDBID:          parseIntID(item.TMDBID),
		TVDBID:          parseIntID(item.TVDBID),
		IMDbID:          normalizeTextID(item.IMDbID),
		Source:          item.Source,
		SourceServiceID: item.SourceServiceID,
		SeasonNumber:    item.SeasonNumber,
		EpisodeNumber:   item.EpisodeNumber,
	}
}

// MatchIdentity returns the strongest safe match between two identities.
func MatchIdentity(a, b Identity) Match {
	if match := matchExternalID(a, b); match.Matched {
		return match
	}

	if match := matchARRID(a, b); match.Matched {
		return match
	}

	return Match{}
}

func matchExternalID(a, b Identity) Match {
	if a.TMDBID != 0 && a.TMDBID == b.TMDBID {
		return Match{
			Matched:  true,
			Reason:   models.CorrelationExternalID,
			Strength: models.CorrelationExact,
			Key:      fmt.Sprintf("tmdb:%d", a.TMDBID),
		}
	}

	if a.TVDBID != 0 && a.TVDBID == b.TVDBID {
		// For episodes, a series-level TVDB ID is not sufficient by itself.
		if a.Kind == models.MediaEpisode || b.Kind == models.MediaEpisode {
			if a.SeasonNumber == 0 ||
				b.SeasonNumber == 0 ||
				a.EpisodeNumber == 0 ||
				b.EpisodeNumber == 0 ||
				a.SeasonNumber != b.SeasonNumber ||
				a.EpisodeNumber != b.EpisodeNumber {
				return Match{}
			}
		}

		return Match{
			Matched:  true,
			Reason:   models.CorrelationExternalID,
			Strength: models.CorrelationExact,
			Key:      fmt.Sprintf("tvdb:%d", a.TVDBID),
		}
	}

	if a.IMDbID != "" && a.IMDbID == b.IMDbID {
		// IMDb IDs on episodic records may identify the episode itself, so
		// an exact non-empty match is safe.
		return Match{
			Matched:  true,
			Reason:   models.CorrelationExternalID,
			Strength: models.CorrelationExact,
			Key:      "imdb:" + a.IMDbID,
		}
	}

	if a.MusicBrainzID != "" &&
		a.MusicBrainzID == b.MusicBrainzID {
		return Match{
			Matched:  true,
			Reason:   models.CorrelationExternalID,
			Strength: models.CorrelationExact,
			Key:      "musicbrainz:" + a.MusicBrainzID,
		}
	}

	return Match{}
}

func matchARRID(a, b Identity) Match {
	if a.SourceServiceID == 0 ||
		b.SourceServiceID == 0 ||
		a.SourceServiceID != b.SourceServiceID {
		return Match{}
	}

	checks := []struct {
		name string
		a    int64
		b    int64
	}{
		{"movie", a.MovieID, b.MovieID},
		{"episode", a.EpisodeID, b.EpisodeID},
		{"series", a.SeriesID, b.SeriesID},
		{"album", a.AlbumID, b.AlbumID},
		{"artist", a.ArtistID, b.ArtistID},
	}

	for _, check := range checks {
		if check.a != 0 && check.a == check.b {
			return Match{
				Matched:  true,
				Reason:   models.CorrelationARRID,
				Strength: models.CorrelationStrong,
				Key: fmt.Sprintf(
					"%s:%d:%d",
					check.name,
					a.SourceServiceID,
					check.a,
				),
			}
		}
	}

	return Match{}
}

func normalizeTextID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func parseIntID(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}

	return id
}

func mediaKindFromRequest(mediaType string) models.MediaKind {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		return models.MediaMovie
	case "tv", "series":
		return models.MediaSeries
	default:
		return ""
	}
}

func mediaKindFromPlayback(mediaType string) models.MediaKind {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "movie":
		return models.MediaMovie
	case "episode":
		return models.MediaEpisode
	case "series", "show":
		return models.MediaSeries
	case "artist":
		return models.MediaArtist
	case "album":
		return models.MediaAlbum
	case "track", "audio":
		return models.MediaTrack
	default:
		return ""
	}
}

// DownloadIdentity contains the normalized identifier used to connect an ARR
// queue record with the corresponding downloader record.
type DownloadIdentity struct {
	ID string
}

// DownloadIdentityFromDownload extracts the cross-service download identifier.
//
// ARR queue records expose DownloadID. Downloader records may expose the same
// identifier as either DownloadID or, for providers whose normalized record ID
// is itself the native download identifier, ID.
func DownloadIdentityFromDownload(item models.Download) DownloadIdentity {
	return DownloadIdentity{
		ID: normalizeDownloadID(item.DownloadID),
	}
}

// MatchDownloadIdentity joins records only when both sides provide the same
// non-empty normalized download identifier.
func MatchDownloadIdentity(a, b models.Download) Match {
	aID := DownloadIdentityFromDownload(a).ID
	bID := DownloadIdentityFromDownload(b).ID

	if aID == "" || bID == "" || aID != bID {
		return Match{}
	}

	return Match{
		Matched:  true,
		Reason:   models.CorrelationDownloadID,
		Strength: models.CorrelationStrong,
		Key:      "download:" + aID,
	}
}

func normalizeDownloadID(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
