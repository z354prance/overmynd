package pipeline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z354prance/undermynd/internal/config"
	"github.com/z354prance/undermynd/internal/job"
	"github.com/z354prance/undermynd/internal/media"
	"github.com/z354prance/undermynd/internal/pipeline"
)

func rvint(n int) []byte {
	if n < 127 {
		return []byte{0x80 | byte(n)}
	}
	return []byte{0x40 | byte(n>>8), byte(n)}
}
func rel(id []byte, body []byte) []byte {
	return append(append(append([]byte{}, id...), rvint(len(body))...), body...)
}

// testMKV is a small Matroska file whose first video track has the given codec id.
func testMKV(codec string, pad int) []byte {
	entry := rel([]byte{0xAE}, append(rel([]byte{0x83}, []byte{1}), rel([]byte{0x86}, []byte(codec))...))
	tracks := rel([]byte{0x16, 0x54, 0xAE, 0x6B}, entry)
	data := rel([]byte{0x1F, 0x43, 0xB6, 0x75}, []byte(strings.Repeat("d", pad)))
	hdr := rel([]byte{0x1A, 0x45, 0xDF, 0xA3}, rel([]byte{0x42, 0x82}, []byte("matroska")))
	return append(hdr, rel([]byte{0x18, 0x53, 0x80, 0x67}, append(tracks, data...))...)
}

// reencodeRig is a Tdarr rig plus a one-file library to re-encode.
func reencodeRig(t *testing.T) (*urig, *fakeTdarr, string, string) {
	u := newURig(t)
	td := newFakeTdarr()
	t.Cleanup(td.srv.Close)
	s := u.cfg.Get()
	s.Tdarr.URL, s.Tdarr.APIKey, s.Tdarr.LibraryID, s.Tdarr.MaxInFlight = td.srv.URL, "tapi_key", "lib1", 2
	if err := u.cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	lib := filepath.Join(t.TempDir(), "Episodes")
	u.eng.LibraryRoots = []string{lib}
	dir := filepath.Join(lib, "Show", "Season 1")
	_ = os.MkdirAll(dir, 0o755)
	orig := filepath.Join(dir, "Show.S01E01.WEBDL-1080p.mkv")
	if err := os.WriteFile(orig, testMKV("V_MPEG4/ISO/AVC", 6000), 0o644); err != nil {
		t.Fatal(err)
	}
	j, err := u.eng.SubmitReencode(orig)
	if err != nil {
		t.Fatal(err)
	}
	u.tick(3)
	if j, _ = u.st.Get(j.ID); j.State != job.QueuedTranscode {
		t.Fatalf("expected the job to be with Tdarr, got %s (%s)", j.State, j.Reason)
	}
	return u, td, j.ID, orig
}

func tdarrOutput(u *urig, id, name string, body []byte) {
	out := filepath.Join(u.cfg.Get().Tdarr.OutDir, id)
	_ = os.MkdirAll(out, 0o755)
	_ = os.WriteFile(filepath.Join(out, name), body, 0o644)
}

func TestReencodeReplacesTheLibraryFileWithAV1(t *testing.T) {
	u, td, id, orig := reencodeRig(t)
	in := filepath.Join(u.cfg.Get().Tdarr.InDir, id, filepath.Base(orig))
	td.mu.Lock()
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Transcode success"}
	td.mu.Unlock()
	if _, err := os.Stat(orig); err != nil {
		t.Fatal("the library file must still be there while Tdarr works")
	}
	av1 := testMKV("V_AV1", 1500)
	tdarrOutput(u, id, filepath.Base(orig), av1)
	u.tick(12)
	j, _ := u.st.Get(id)
	if j.State != job.Cleaned || !strings.Contains(j.Reason, "replaced with AV1") {
		t.Fatalf("expected the library file replaced, got %s (%s)", j.State, j.Reason)
	}
	if c, err := media.VideoCodec(orig); err != nil || c != "av1" {
		t.Fatalf("the library file must now be AV1: %q %v", c, err)
	}
	if b, _ := os.ReadFile(orig); string(b) != string(av1) {
		t.Fatal("the library file must be Tdarr's output")
	}
	ents, _ := os.ReadDir(filepath.Dir(orig))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".undermynd-") {
			t.Fatalf("a temporary file was left in the library: %s", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(u.cfg.Get().Tdarr.InDir, id)); err == nil {
		t.Fatal("the Tdarr input folder should be cleaned up")
	}
}

func TestReencodeNeverReplacesWithANonAV1Output(t *testing.T) {
	u, td, id, orig := reencodeRig(t)
	in := filepath.Join(u.cfg.Get().Tdarr.InDir, id, filepath.Base(orig))
	td.mu.Lock()
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Transcode success"}
	td.mu.Unlock()
	before, _ := os.ReadFile(orig)
	tdarrOutput(u, id, filepath.Base(orig), testMKV("V_MPEGH/ISO/HEVC", 1500))
	u.tick(12)
	j, _ := u.st.Get(id)
	if j.State == job.Cleaned && strings.Contains(j.Reason, "replaced") {
		t.Fatalf("a non-AV1 output must not replace the library file: %s", j.Reason)
	}
	after, _ := os.ReadFile(orig)
	if string(before) != string(after) {
		t.Fatal("the library file must be untouched")
	}
}

func TestReencodeTdarrErrorLeavesTheLibraryFileAlone(t *testing.T) {
	u, td, id, orig := reencodeRig(t)
	fa := &failArr{}
	u.eng.ArrFor = func(config.Category) pipeline.ImportChecker { return fa }
	in := filepath.Join(u.cfg.Get().Tdarr.InDir, id, filepath.Base(orig))
	td.mu.Lock()
	td.records[in] = map[string]any{"_id": in, "TranscodeDecisionMaker": "Transcode error"}
	td.mu.Unlock()
	u.tick(4)
	j, _ := u.st.Get(id)
	if j.State == job.Ready || j.State == job.Cleaned && strings.Contains(j.Reason, "replaced") {
		t.Fatalf("an error must never hand anything back, got %s (%s)", j.State, j.Reason)
	}
	if c, _ := media.VideoCodec(orig); c != "h264" {
		t.Fatalf("the library file must be untouched, codec %q", c)
	}
	if len(fa.told) != 0 {
		t.Fatalf("the arr must not be told anything about a re-encode, got %v", fa.told)
	}
	if j.State != job.Cleaned && j.State != job.Failed {
		t.Fatalf("a failed re-encode should end, got %s (%s)", j.State, j.Reason)
	}
}

func TestReencodeRefusesFilesOutsideTheLibrary(t *testing.T) {
	u := newURig(t)
	lib := t.TempDir()
	u.eng.LibraryRoots = []string{lib}
	outside := filepath.Join(t.TempDir(), "x.mkv")
	_ = os.WriteFile(outside, testMKV("V_AV1", 10), 0o644)
	if _, err := u.eng.SubmitReencode(outside); err == nil {
		t.Fatal("a file outside the library must be refused")
	}
	if _, err := u.eng.SubmitReencode(filepath.Join(lib, "..", filepath.Base(filepath.Dir(outside)), "x.mkv")); err == nil {
		t.Fatal("a path that climbs out of the library must be refused")
	}
}
