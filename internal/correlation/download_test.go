package correlation

import (
	"testing"

	"github.com/z354prance/overmynd/internal/models"
)

func TestARRAndQBittorrentDownloadIDMatch(t *testing.T) {
	arr := models.Download{
		Source:     models.ServiceSonarr,
		DownloadID: "ABCDEF1234567890",
	}

	qbit := models.Download{
		Source:     models.ServiceQBittorrent,
		DownloadID: "abcdef1234567890",
	}

	match := MatchDownloadIdentity(arr, qbit)

	if !match.Matched {
		t.Fatal("ARR and qBittorrent download IDs should match")
	}
	if match.Reason != models.CorrelationDownloadID {
		t.Fatalf("reason = %q", match.Reason)
	}
	if match.Strength != models.CorrelationStrong {
		t.Fatalf("strength = %q", match.Strength)
	}
	if match.Key != "download:abcdef1234567890" {
		t.Fatalf("key = %q", match.Key)
	}
}

func TestARRAndNZBGetDownloadIDMatch(t *testing.T) {
	arr := models.Download{
		Source:     models.ServiceRadarr,
		DownloadID: "NZB-12345",
	}

	nzb := models.Download{
		Source:     models.ServiceNZBGet,
		DownloadID: "nzb-12345",
	}

	match := MatchDownloadIdentity(arr, nzb)

	if !match.Matched {
		t.Fatal("ARR and NZBGet download IDs should match")
	}
	if match.Reason != models.CorrelationDownloadID {
		t.Fatalf("reason = %q", match.Reason)
	}
}

func TestDownloadIDDoesNotMatchEmptyValues(t *testing.T) {
	a := models.Download{
		Source: models.ServiceSonarr,
	}
	b := models.Download{
		Source: models.ServiceQBittorrent,
	}

	if match := MatchDownloadIdentity(a, b); match.Matched {
		t.Fatalf("empty download IDs unexpectedly matched: %#v", match)
	}
}

func TestDifferentDownloadIDsDoNotMatch(t *testing.T) {
	a := models.Download{
		Source:     models.ServiceSonarr,
		DownloadID: "aaa",
	}
	b := models.Download{
		Source:     models.ServiceQBittorrent,
		DownloadID: "bbb",
	}

	if match := MatchDownloadIdentity(a, b); match.Matched {
		t.Fatalf("different download IDs unexpectedly matched: %#v", match)
	}
}

func TestDownloaderExplicitDownloadIDPreferred(t *testing.T) {
	a := models.Download{
		Source:     models.ServiceRadarr,
		DownloadID: "native-id",
	}

	b := models.Download{
		Source:     models.ServiceNZBGet,
		ID:         "fallback-id",
		DownloadID: "native-id",
	}

	if match := MatchDownloadIdentity(a, b); !match.Matched {
		t.Fatal("explicit downloader DownloadID should be preferred")
	}
}
