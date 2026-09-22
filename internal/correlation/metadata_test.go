package correlation

import (
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestMovieMetadataMatch(t *testing.T) {
	a := MetadataIdentity{
		Kind:  models.MediaMovie,
		Title: normalizeTitle("The Example Movie"),
		Year:  2026,
	}
	b := MetadataIdentity{
		Kind:  models.MediaMovie,
		Title: normalizeTitle("The.Example.Movie"),
		Year:  2026,
	}

	match := MatchMetadata(a, b)

	if !match.Matched {
		t.Fatal("same normalized movie metadata should match")
	}
	if match.Reason != models.CorrelationMetadata {
		t.Fatalf("reason = %q", match.Reason)
	}
	if match.Strength != models.CorrelationWeak {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestMoviesWithDifferentYearsDoNotMatch(t *testing.T) {
	a := MetadataIdentity{
		Kind:  models.MediaMovie,
		Title: normalizeTitle("Example"),
		Year:  1999,
	}
	b := MetadataIdentity{
		Kind:  models.MediaMovie,
		Title: normalizeTitle("Example"),
		Year:  2026,
	}

	if match := MatchMetadata(a, b); match.Matched {
		t.Fatalf("different movie years unexpectedly matched: %#v", match)
	}
}

func TestEpisodeMetadataRequiresCoordinates(t *testing.T) {
	a := MetadataIdentity{
		Kind:          models.MediaEpisode,
		Title:         normalizeTitle("WondLa"),
		SeasonNumber:  1,
		EpisodeNumber: 3,
	}
	b := MetadataIdentity{
		Kind:          models.MediaEpisode,
		Title:         normalizeTitle("wondla"),
		SeasonNumber:  1,
		EpisodeNumber: 3,
	}

	if match := MatchMetadata(a, b); !match.Matched {
		t.Fatal("same show/season/episode should match")
	}

	b.EpisodeNumber = 4

	if match := MatchMetadata(a, b); match.Matched {
		t.Fatalf("different episodes unexpectedly matched: %#v", match)
	}
}

func TestEpisodeWithoutCoordinatesDoesNotMatch(t *testing.T) {
	a := MetadataIdentity{
		Kind:  models.MediaEpisode,
		Title: normalizeTitle("WondLa"),
	}
	b := MetadataIdentity{
		Kind:  models.MediaEpisode,
		Title: normalizeTitle("WondLa"),
	}

	if match := MatchMetadata(a, b); match.Matched {
		t.Fatalf("coordinate-less episodes unexpectedly matched: %#v", match)
	}
}

func TestDifferentKnownKindsDoNotMatch(t *testing.T) {
	a := MetadataIdentity{
		Kind:  models.MediaMovie,
		Title: normalizeTitle("Example"),
	}
	b := MetadataIdentity{
		Kind:  models.MediaSeries,
		Title: normalizeTitle("Example"),
	}

	if match := MatchMetadata(a, b); match.Matched {
		t.Fatalf("different media kinds unexpectedly matched: %#v", match)
	}
}

func TestPlaybackEpisodeUsesShowTitle(t *testing.T) {
	session := models.PlaybackSession{
		MediaType:     "episode",
		MediaTitle:    "Chapter 3: Bargain",
		ShowTitle:     "WondLa",
		SeasonNumber:  1,
		EpisodeNumber: 3,
	}

	got := MetadataFromPlayback(session)

	if got.Title != "wondla" {
		t.Fatalf("title = %q, want wondla", got.Title)
	}
	if got.Kind != models.MediaEpisode {
		t.Fatalf("kind = %q", got.Kind)
	}
}

func TestTitleNormalization(t *testing.T) {
	a := normalizeTitle("  WondLa.S01-E03  ")
	b := normalizeTitle("wondla s01 e03")

	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
}
