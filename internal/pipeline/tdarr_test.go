package pipeline_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/z354prance/undermynd/internal/job"
)

var mkvHead = "\x1a\x45\xdf\xa3\x01\x00\x00\x00"

func TestTdarrStage(t *testing.T) {
	u := newURig(t)
	s := u.cfg.Get()
	s.Categories[0].Transcode = true
	if err := u.cfg.Set(s); err != nil {
		t.Fatal(err)
	}
	id := u.addNZB("Tdarr.Show.S01E01")
	u.tick(2)
	body := mkvHead + strings.Repeat("v", 4000)
	u.tb.finish("101", map[string]string{
		"Tdarr.Show.S01E01/Tdarr.Show.S01E01.mkv": body,
		"Tdarr.Show.S01E01/Tdarr.Show.S01E01.srt": "subs",
	})
	u.tick(3)
	j, _ := u.st.Get(id)
	if j.State != job.QueuedTranscode {
		t.Fatalf("expected queued_transcode, got %s (%s)", j.State, j.Reason)
	}
	in := filepath.Join(s.Tdarr.InDir, id, "Tdarr.Show.S01E01.mkv")
	if !exists(in) {
		t.Fatalf("video should be in the Tdarr input folder: %s", in)
	}
	u.tick(3)
	if j, _ = u.st.Get(id); j.State != job.QueuedTranscode {
		t.Fatalf("must keep waiting without output, got %s", j.State)
	}
	// Tdarr writes a smaller transcode, with a different container
	out := filepath.Join(s.Tdarr.OutDir, id)
	_ = os.MkdirAll(out, 0o755)
	small := mkvHead + strings.Repeat("s", 1500)
	_ = os.WriteFile(filepath.Join(out, "Tdarr.Show.S01E01.mkv"), []byte(small), 0o644)
	u.tick(1)
	if j, _ = u.st.Get(id); j.State != job.Transcoding {
		t.Fatalf("expected transcoding, got %s (%s)", j.State, j.Reason)
	}
	u.tick(4)
	j, _ = u.st.Get(id)
	if j.State != job.Ready {
		t.Fatalf("expected ready, got %s (%s)", j.State, j.Reason)
	}
	got, _ := os.ReadFile(filepath.Join(j.CompleteDir, "Tdarr.Show.S01E01.mkv"))
	if string(got) != small {
		t.Fatal("the transcoded file must be what is handed back")
	}
	if !exists(filepath.Join(j.CompleteDir, "Tdarr.Show.S01E01.srt")) {
		t.Fatal("subtitles must be kept")
	}
	if exists(filepath.Join(s.Tdarr.InDir, id)) || exists(out) {
		t.Fatal("Tdarr folders must be cleared")
	}
}

func TestTdarrBadOutputFails(t *testing.T) {
	u := newURig(t)
	s := u.cfg.Get()
	s.Categories[0].Transcode = true
	_ = u.cfg.Set(s)
	id := u.addNZB("Bad.Tdarr.S01E01")
	u.tick(2)
	u.tb.finish("101", map[string]string{"Bad.Tdarr.S01E01/Bad.Tdarr.S01E01.mkv": mkvHead + strings.Repeat("v", 4000)})
	u.tick(3)
	out := filepath.Join(s.Tdarr.OutDir, id)
	_ = os.MkdirAll(out, 0o755)
	_ = os.WriteFile(filepath.Join(out, "Bad.Tdarr.S01E01.mkv"), []byte("garbage that is not matroska"), 0o644)
	u.tick(6)
	j, _ := u.st.Get(id)
	if j.State != job.Failed {
		t.Fatalf("expected failed, got %s (%s)", j.State, j.Reason)
	}
	u.tick(2)
	if exists(filepath.Join(s.Tdarr.InDir, id)) || exists(out) {
		t.Fatal("a failed job must clear the Tdarr folders")
	}
}
