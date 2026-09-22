package correlation

import (
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestEngineBuildsTransitiveEpisodeLifecycle(t *testing.T) {
	engine := New()

	input := Input{
		Attention: []models.AttentionItem{
			{
				ID:              "sonarr-attention",
				Source:          models.ServiceSonarr,
				SourceServiceID: 2,
				SourceID:        77,
				State:           models.AttentionInProgress,
				Kind:            models.MediaEpisode,
				Title:           "WondLa",
				Year:            2024,
				SeasonNumber:    1,
				EpisodeNumber:   3,
				TVDBID:          10565927,
			},
		},
		Downloads: []models.Download{
			{
				ID:              "sonarr-queue",
				Source:          models.ServiceSonarr,
				SourceServiceID: 2,
				Title:           "WondLa.S01E03",
				Status:          "downloading",
				DownloadID:      "ABC123",
				OutputPath:      "/downloads/tv/WondLa/WondLa.S01E03.mkv",
				EpisodeID:       77,
				SeriesID:        12,
				Size:            1000,
				SizeLeft:        500,
			},
			{
				ID:              "qbit-record",
				Source:          models.ServiceQBittorrent,
				SourceServiceID: 4,
				Title:           "WondLa.S01E03",
				Status:          "downloading",
				DownloadID:      "abc123",
				OutputPath:      "/downloads/tv/WondLa/WondLa.S01E03.mkv",
				Size:            1000,
				SizeLeft:        500,
			},
		},
		Processing: []models.ProcessingJob{
			{
				ID:              "tdarr-record",
				Source:          models.ServiceTdarr,
				SourceServiceID: 7,
				Title:           "WondLa.S01E03.mkv",
				Path:            "/processing/tv/WondLa/WondLa.S01E03.mkv",
				State:           models.ProcessingStateQueued,
				Stage:           "transcode",
			},
		},
		Playback: []models.PlaybackSession{
			{
				ID:              "tracearr-record",
				Source:          models.ServiceTracearr,
				SourceServiceID: 8,
				MediaTitle:      "Chapter 3: Bargain",
				MediaType:       "episode",
				ShowTitle:       "WondLa",
				SeasonNumber:    1,
				EpisodeNumber:   3,
				Year:            2024,
				TVDBID:          "10565927",
				State:           "playing",
			},
		},
	}

	got := engine.Build(input)

	if len(got) != 1 {
		t.Fatalf("lifecycles = %d, want 1: %#v", len(got), got)
	}

	lifecycle := got[0]

	if lifecycle.Title != "WondLa" {
		t.Fatalf("title = %q, want WondLa", lifecycle.Title)
	}

	if lifecycle.Kind != models.MediaEpisode {
		t.Fatalf(
			"kind = %q, want %q",
			lifecycle.Kind,
			models.MediaEpisode,
		)
	}

	if lifecycle.TVDBID != 10565927 {
		t.Fatalf("TVDBID = %d, want 10565927", lifecycle.TVDBID)
	}

	if lifecycle.SeasonNumber != 1 ||
		lifecycle.EpisodeNumber != 3 {
		t.Fatalf(
			"episode = S%02dE%02d, want S01E03",
			lifecycle.SeasonNumber,
			lifecycle.EpisodeNumber,
		)
	}

	if lifecycle.Stage != models.LifecycleStagePlaying {
		t.Fatalf(
			"stage = %q, want %q",
			lifecycle.Stage,
			models.LifecycleStagePlaying,
		)
	}

	if len(lifecycle.References) != 5 {
		t.Fatalf(
			"references = %d, want 5",
			len(lifecycle.References),
		)
	}
}

func TestEngineKeepsUnrelatedItemsSeparate(t *testing.T) {
	engine := New()

	input := Input{
		Attention: []models.AttentionItem{
			{
				ID:              "episode-3",
				Source:          models.ServiceSonarr,
				SourceServiceID: 2,
				SourceID:        3,
				Kind:            models.MediaEpisode,
				Title:           "WondLa",
				SeasonNumber:    1,
				EpisodeNumber:   3,
				TVDBID:          100,
			},
			{
				ID:              "episode-4",
				Source:          models.ServiceSonarr,
				SourceServiceID: 2,
				SourceID:        4,
				Kind:            models.MediaEpisode,
				Title:           "WondLa",
				SeasonNumber:    1,
				EpisodeNumber:   4,
				TVDBID:          100,
			},
		},
	}

	got := engine.Build(input)

	if len(got) != 2 {
		t.Fatalf("lifecycles = %d, want 2: %#v", len(got), got)
	}
}

func TestEngineCorrelatesSeerrMovieByTMDB(t *testing.T) {
	engine := New()

	input := Input{
		Requests: []models.Request{
			{
				ID:              "request-1",
				Source:          models.ServiceSeerr,
				SourceServiceID: 6,
				SourceID:        1,
				MediaType:       "movie",
				TMDBID:          12345,
				Status:          "approved",
			},
		},
		Attention: []models.AttentionItem{
			{
				ID:              "radarr-1",
				Source:          models.ServiceRadarr,
				SourceServiceID: 1,
				SourceID:        42,
				State:           models.AttentionMissing,
				Kind:            models.MediaMovie,
				Title:           "Example Movie",
				Year:            2026,
				TMDBID:          12345,
			},
		},
	}

	got := engine.Build(input)

	if len(got) != 1 {
		t.Fatalf("lifecycles = %d, want 1", len(got))
	}

	if len(got[0].References) != 2 {
		t.Fatalf(
			"references = %d, want 2",
			len(got[0].References),
		)
	}

	if got[0].TMDBID != 12345 {
		t.Fatalf("TMDBID = %d, want 12345", got[0].TMDBID)
	}

	if got[0].Stage != models.LifecycleStageWanted {
		t.Fatalf(
			"stage = %q, want %q",
			got[0].Stage,
			models.LifecycleStageWanted,
		)
	}

	if len(got[0].Problems) != 1 ||
		got[0].Problems[0] != models.LifecycleProblemMissing {
		t.Fatalf("problems = %#v", got[0].Problems)
	}
}

func TestEngineWeakMetadataCannotMergeDifferentAuthoritativeMovies(t *testing.T) {
	engine := New()

	input := Input{
		Attention: []models.AttentionItem{
			{
				ID:              "movie-a",
				Source:          models.ServiceRadarr,
				SourceServiceID: 1,
				SourceID:        10,
				Kind:            models.MediaMovie,
				Title:           "The Thing",
				Year:            1982,
				TMDBID:          1091,
			},
			{
				ID:              "movie-b",
				Source:          models.ServiceRadarr,
				SourceServiceID: 9,
				SourceID:        20,
				Kind:            models.MediaMovie,
				Title:           "The Thing",
				Year:            1982,
				TMDBID:          999999,
			},
		},
	}

	got := engine.Build(input)

	if len(got) != 2 {
		t.Fatalf(
			"weak metadata merged different authoritative movies: %#v",
			got,
		)
	}
}

func TestEngineWeakFilenameCanAttachProcessingRecord(t *testing.T) {
	engine := New()

	input := Input{
		Downloads: []models.Download{
			{
				ID:              "download-1",
				Source:          models.ServiceSonarr,
				SourceServiceID: 2,
				Title:           "episode.mkv",
				OutputPath:      "/downloads/show/episode.mkv",
				EpisodeID:       77,
			},
		},
		Processing: []models.ProcessingJob{
			{
				ID:              "processing-1",
				Source:          models.ServiceTdarr,
				SourceServiceID: 7,
				Title:           "episode.mkv",
				Path:            "/different/root/episode.mkv",
				State:           models.ProcessingStateQueued,
			},
		},
	}

	got := engine.Build(input)

	if len(got) != 1 {
		t.Fatalf(
			"weak filename should attach processing record; lifecycles = %d",
			len(got),
		)
	}

	if len(got[0].References) != 2 {
		t.Fatalf(
			"references = %d, want 2",
			len(got[0].References),
		)
	}
}

func TestCompletedARRWarningIsImportBlocked(t *testing.T) {
	item := models.Download{
		Source:          models.ServiceSonarr,
		SourceServiceID: 2,
		Status:          "completed",
		TrackedDownload: "warning",
		Size:            1206697984,
		SizeLeft:        0,
	}

	if got := downloadStage(item); got != models.LifecycleStageImporting {
		t.Fatalf(
			"stage = %q, want %q",
			got,
			models.LifecycleStageImporting,
		)
	}

	problems := downloadProblems(item)

	if len(problems) != 1 {
		t.Fatalf("problems = %#v, want one problem", problems)
	}

	if problems[0] != models.LifecycleProblemImportBlocked {
		t.Fatalf(
			"problem = %q, want %q",
			problems[0],
			models.LifecycleProblemImportBlocked,
		)
	}
}

func TestHealthyCompletedDownloadRemainsDownloaded(t *testing.T) {
	item := models.Download{
		Source:          models.ServiceSonarr,
		SourceServiceID: 2,
		Status:          "completed",
		Size:            1000,
		SizeLeft:        0,
	}

	if got := downloadStage(item); got != models.LifecycleStageDownloaded {
		t.Fatalf(
			"stage = %q, want %q",
			got,
			models.LifecycleStageDownloaded,
		)
	}

	if problems := downloadProblems(item); len(problems) != 0 {
		t.Fatalf(
			"healthy completed download has problems: %#v",
			problems,
		)
	}
}

func TestStandaloneRequestPreservesTitle(t *testing.T) {
	engine := New()

	input := Input{
		Requests: []models.Request{
			{
				ID:              "6:request:42",
				Source:          models.ServiceSeerr,
				SourceServiceID: 6,
				SourceID:        42,
				MediaType:       "tv",
				Title:           "Example TV Series",
				TMDBID:          12345,
				Status:          "approved",
				MediaStatus:     "processing",
			},
		},
	}

	got := engine.Build(input)

	if len(got) != 1 {
		t.Fatalf("lifecycles = %d, want 1", len(got))
	}

	if got[0].Title != "Example TV Series" {
		t.Fatalf(
			"title = %q, want %q",
			got[0].Title,
			"Example TV Series",
		)
	}

	if got[0].Stage != models.LifecycleStageRequested {
		t.Fatalf(
			"stage = %q, want %q",
			got[0].Stage,
			models.LifecycleStageRequested,
		)
	}

	if got[0].TMDBID != 12345 {
		t.Fatalf("TMDBID = %d, want 12345", got[0].TMDBID)
	}
}
