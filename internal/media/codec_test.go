package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func vint(n int) []byte { // size with a 1-byte marker (n < 127) or 2-byte
	if n < 127 {
		return []byte{0x80 | byte(n)}
	}
	return []byte{0x40 | byte(n>>8), byte(n)}
}

func el(id []byte, body []byte) []byte {
	return append(append(append([]byte{}, id...), vint(len(body))...), body...)
}

func mkv(codec string, withAudioFirst bool) []byte {
	entry := func(typ byte, c string) []byte {
		return el([]byte{0xAE}, append(el([]byte{0x83}, []byte{typ}), el([]byte{0x86}, []byte(c))...))
	}
	var tracks []byte
	if withAudioFirst {
		tracks = append(tracks, entry(2, "A_OPUS")...)
	}
	tracks = append(tracks, entry(1, codec)...)
	seg := append(el([]byte{0x15, 0x49, 0xA9, 0x66}, []byte("info")), el([]byte{0x16, 0x54, 0xAE, 0x6B}, tracks)...)
	seg = append(seg, el([]byte{0x1F, 0x43, 0xB6, 0x75}, []byte("data"))...)
	hdr := el([]byte{0x1A, 0x45, 0xDF, 0xA3}, el([]byte{0x42, 0x82}, []byte("matroska")))
	return append(hdr, el([]byte{0x18, 0x53, 0x80, 0x67}, seg)...)
}

func TestVideoCodecMatroska(t *testing.T) {
	dir := t.TempDir()
	for codec, want := range map[string]string{"V_AV1": "av1", "V_MPEG4/ISO/AVC": "h264", "V_MPEGH/ISO/HEVC": "hevc", "V_VP9": "vp9"} {
		p := filepath.Join(dir, "x.mkv")
		_ = os.WriteFile(p, mkv(codec, true), 0o644)
		got, err := VideoCodec(p)
		if err != nil || got != want {
			t.Fatalf("%s: got %q %v want %q", codec, got, err, want)
		}
	}
}

func TestVideoCodecMP4AndErrors(t *testing.T) {
	dir := t.TempDir()
	ftyp := append([]byte{0, 0, 0, 16}, []byte("ftypisom\x00\x00\x00\x00")...)
	moov := append([]byte{0, 0, 0, 24}, []byte("moov....mp4a....av01")...)
	p := filepath.Join(dir, "a.mp4")
	_ = os.WriteFile(p, append(ftyp, moov...), 0o644)
	if c, err := VideoCodec(p); err != nil || c != "av1" {
		t.Fatalf("mp4 av1: %q %v", c, err)
	}
	q := filepath.Join(dir, "b.mkv")
	_ = os.WriteFile(q, []byte("not a video at all"), 0o644)
	if _, err := VideoCodec(q); err == nil {
		t.Fatal("garbage must be an error")
	}
	if _, err := VideoCodec(filepath.Join(dir, "missing.mkv")); err == nil {
		t.Fatal("missing file must be an error")
	}
}

func TestScanFindsNonAV1SinceDate(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "Episodes", "Show")
	_ = os.MkdirAll(show, 0o755)
	write := func(name string, b []byte, age time.Duration) string {
		p := filepath.Join(show, name)
		_ = os.WriteFile(p, b, 0o644)
		ts := time.Now().Add(-age)
		_ = os.Chtimes(p, ts, ts)
		return p
	}
	write("new-h264.mkv", mkv("V_MPEG4/ISO/AVC", false), time.Hour)
	write("new-av1.mkv", mkv("V_AV1", false), time.Hour)
	write("junk.mkv", []byte("garbage garbage"), time.Hour)
	write("readme.txt", []byte("x"), time.Hour)
	// content time is old, but the inode change time is now: it still counts as recent
	got := Scan([]string{root}, time.Now().Add(-24*time.Hour))
	names := map[string]string{}
	for _, f := range got {
		names[filepath.Base(f.Path)] = f.Codec
	}
	if len(names) != 2 || names["new-h264.mkv"] != "h264" || names["junk.mkv"] != "unknown" {
		t.Fatalf("scan found %v", names)
	}
	if future := Scan([]string{root}, time.Now().Add(time.Hour)); len(future) != 0 {
		t.Fatalf("nothing is newer than the future, got %v", future)
	}
}
