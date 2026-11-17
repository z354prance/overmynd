package correlation

import (
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestMatchIdentityTMDB(t *testing.T) {
	a := Identity{
		Kind:   models.MediaMovie,
		TMDBID: 12345,
	}
	b := Identity{
		Kind:   models.MediaMovie,
		TMDBID: 12345,
	}

	match := MatchIdentity(a, b)

	if !match.Matched {
		t.Fatal("expected TMDB identities to match")
	}
	if match.Reason != models.CorrelationExternalID {
		t.Fatalf("reason = %q", match.Reason)
	}
	if match.Strength != models.CorrelationExact {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestEpisodeTVDBRequiresEpisodeCoordinates(t *testing.T) {
	a := Identity{
		Kind:          models.MediaEpisode,
		TVDBID:        100,
		SeasonNumber:  1,
		EpisodeNumber: 3,
	}
	b := Identity{
		Kind:          models.MediaEpisode,
		TVDBID:        100,
		SeasonNumber:  1,
		EpisodeNumber: 4,
	}

	if match := MatchIdentity(a, b); match.Matched {
		t.Fatalf("different episodes unexpectedly matched: %#v", match)
	}

	b.EpisodeNumber = 3

	match := MatchIdentity(a, b)
	if !match.Matched {
		t.Fatal("same TVDB series/season/episode should match")
	}
	if match.Strength != models.CorrelationExact {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestARRIDRequiresSameService(t *testing.T) {
	a := Identity{
		Source:          models.ServiceSonarr,
		SourceServiceID: 2,
		EpisodeID:       77,
	}
	b := Identity{
		Source:          models.ServiceSonarr,
		SourceServiceID: 2,
		EpisodeID:       77,
	}

	match := MatchIdentity(a, b)
	if !match.Matched {
		t.Fatal("same service-scoped ARR ID should match")
	}
	if match.Reason != models.CorrelationARRID {
		t.Fatalf("reason = %q", match.Reason)
	}
	if match.Strength != models.CorrelationStrong {
		t.Fatalf("strength = %q", match.Strength)
	}

	b.SourceServiceID = 99

	if match := MatchIdentity(a, b); match.Matched {
		t.Fatalf(
			"ARR IDs from different service instances unexpectedly matched: %#v",
			match,
		)
	}
}

func TestPlaybackNumericTVDBID(t *testing.T) {
	session := models.PlaybackSession{
		Source:          models.ServiceTracearr,
		SourceServiceID: 8,
		MediaType:       "episode",
		TVDBID:          "10565927",
		SeasonNumber:    1,
		EpisodeNumber:   3,
	}

	identity := IdentityFromPlayback(session)

	if identity.TVDBID != 10565927 {
		t.Fatalf(
			"TVDBID = %d, want 10565927",
			identity.TVDBID,
		)
	}
	if identity.Kind != models.MediaEpisode {
		t.Fatalf(
			"Kind = %q, want %q",
			identity.Kind,
			models.MediaEpisode,
		)
	}
}

func TestIMDbNormalization(t *testing.T) {
	a := Identity{
		IMDbID: "TT1234567",
	}
	b := Identity{
		IMDbID: " tt1234567 ",
	}

	a.IMDbID = normalizeTextID(a.IMDbID)
	b.IMDbID = normalizeTextID(b.IMDbID)

	match := MatchIdentity(a, b)
	if !match.Matched {
		t.Fatal("normalized IMDb IDs should match")
	}
}

func TestAttentionARRIdentity(t *testing.T) {
	tests := []struct {
		name   string
		item   models.AttentionItem
		wantID int64
		actual func(Identity) int64
	}{
		{
			name: "radarr movie",
			item: models.AttentionItem{
				Source:          models.ServiceRadarr,
				SourceServiceID: 1,
				SourceID:        42,
				Kind:            models.MediaMovie,
			},
			wantID: 42,
			actual: func(identity Identity) int64 {
				return identity.MovieID
			},
		},
		{
			name: "sonarr episode",
			item: models.AttentionItem{
				Source:          models.ServiceSonarr,
				SourceServiceID: 2,
				SourceID:        77,
				Kind:            models.MediaEpisode,
			},
			wantID: 77,
			actual: func(identity Identity) int64 {
				return identity.EpisodeID
			},
		},
		{
			name: "lidarr album",
			item: models.AttentionItem{
				Source:          models.ServiceLidarr,
				SourceServiceID: 3,
				SourceID:        99,
				Kind:            models.MediaAlbum,
			},
			wantID: 99,
			actual: func(identity Identity) int64 {
				return identity.AlbumID
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity := IdentityFromAttention(test.item)

			if got := test.actual(identity); got != test.wantID {
				t.Fatalf("ARR ID = %d, want %d", got, test.wantID)
			}
		})
	}
}

func TestAttentionMatchesARRQueueBySourceID(t *testing.T) {
	attention := models.AttentionItem{
		Source:          models.ServiceSonarr,
		SourceServiceID: 2,
		SourceID:        77,
		Kind:            models.MediaEpisode,
	}

	download := models.Download{
		Source:          models.ServiceSonarr,
		SourceServiceID: 2,
		EpisodeID:       77,
	}

	match := MatchIdentity(
		IdentityFromAttention(attention),
		IdentityFromDownload(download),
	)

	if !match.Matched {
		t.Fatal("Sonarr attention item should match its queue record")
	}
	if match.Reason != models.CorrelationARRID {
		t.Fatalf("reason = %q", match.Reason)
	}
	if match.Strength != models.CorrelationStrong {
		t.Fatalf("strength = %q", match.Strength)
	}
}
