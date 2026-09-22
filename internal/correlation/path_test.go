package correlation

import (
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestPathMatchAcrossDifferentMountRoots(t *testing.T) {
	download := models.Download{
		OutputPath: "/downloads/tv/WondLa/WondLa.S01E03.mkv",
	}

	job := models.ProcessingJob{
		Path: "/media-processing/tv/WondLa/WondLa.S01E03.mkv",
	}

	match := MatchDownloadProcessingPath(download, job)

	if !match.Matched {
		t.Fatal("matching path tails should correlate")
	}
	if match.Reason != models.CorrelationPath {
		t.Fatalf("reason = %q", match.Reason)
	}
	if match.Strength != models.CorrelationMedium {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestFilenameOnlyPathMatchIsWeak(t *testing.T) {
	download := models.Download{
		OutputPath: "/downloads/a/movie.mkv",
	}

	job := models.ProcessingJob{
		Path: "/processing/b/movie.mkv",
	}

	match := MatchDownloadProcessingPath(download, job)

	if !match.Matched {
		t.Fatal("same filename should provide weak correlation evidence")
	}
	if match.Strength != models.CorrelationWeak {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestDifferentFilenamesDoNotMatch(t *testing.T) {
	download := models.Download{
		OutputPath: "/downloads/tv/WondLa/episode3.mkv",
	}

	job := models.ProcessingJob{
		Path: "/processing/tv/WondLa/episode4.mkv",
	}

	if match := MatchDownloadProcessingPath(download, job); match.Matched {
		t.Fatalf("different files unexpectedly matched: %#v", match)
	}
}

func TestPathMatchIsCaseInsensitive(t *testing.T) {
	download := models.Download{
		OutputPath: "/Downloads/TV/WondLa/Episode.mkv",
	}

	job := models.ProcessingJob{
		Path: "/processing/tv/wondla/episode.MKV",
	}

	match := MatchDownloadProcessingPath(download, job)

	if !match.Matched {
		t.Fatal("case differences should not prevent path correlation")
	}
	if match.Strength != models.CorrelationMedium {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestWindowsAndUnixSeparatorsMatch(t *testing.T) {
	download := models.Download{
		OutputPath: `D:\downloads\WondLa\episode.mkv`,
	}

	job := models.ProcessingJob{
		Path: "/processing/WondLa/episode.mkv",
	}

	match := MatchDownloadProcessingPath(download, job)

	if !match.Matched {
		t.Fatal("separator differences should not prevent correlation")
	}
	if match.Strength != models.CorrelationMedium {
		t.Fatalf("strength = %q", match.Strength)
	}
}

func TestEmptyPathsDoNotMatch(t *testing.T) {
	if match := MatchDownloadProcessingPath(
		models.Download{},
		models.ProcessingJob{},
	); match.Matched {
		t.Fatalf("empty paths unexpectedly matched: %#v", match)
	}
}
