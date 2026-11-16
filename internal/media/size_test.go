package media

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVideoSizeAndTier(t *testing.T) {
	be := func(n int) []byte { return []byte{byte(n >> 8), byte(n)} }
	video := el([]byte{0xE0}, append(el([]byte{0xB0}, be(1920)), el([]byte{0xBA}, be(800))...))
	audio := el([]byte{0xAE}, append(el([]byte{0x83}, []byte{2}), el([]byte{0x86}, []byte("A_OPUS"))...))
	entry := el([]byte{0xAE}, append(append(el([]byte{0x83}, []byte{1}), el([]byte{0x86}, []byte("V_AV1"))...), video...))
	tracks := el([]byte{0x16, 0x54, 0xAE, 0x6B}, append(audio, entry...))
	seg := append(tracks, el([]byte{0x1F, 0x43, 0xB6, 0x75}, []byte("data"))...)
	hdr := el([]byte{0x1A, 0x45, 0xDF, 0xA3}, el([]byte{0x42, 0x82}, []byte("matroska")))
	p := filepath.Join(t.TempDir(), "a.mkv")
	_ = os.WriteFile(p, append(hdr, el([]byte{0x18, 0x53, 0x80, 0x67}, seg)...), 0o644)
	w, h, err := VideoSize(p)
	if err != nil || w != 1920 || h != 800 {
		t.Fatalf("got %dx%d %v", w, h, err)
	}
	for _, c := range []struct{ w, h, want int }{{3840, 1600, 2160}, {1920, 800, 1080}, {1440, 1080, 1080}, {1280, 544, 720}, {640, 480, 480}, {720, 576, 480}, {480, 360, 360}, {426, 240, 360}} {
		if got := Tier(c.w, c.h); got != c.want {
			t.Fatalf("%dx%d: tier %d want %d", c.w, c.h, got, c.want)
		}
	}
}

func TestVideoSizeMP4(t *testing.T) {
	tkhd := make([]byte, 92) // box size+type, version 0, then fields up to width and height
	copy(tkhd[4:], "tkhd")
	tkhd[0], tkhd[3] = 0, 92
	tkhd[84], tkhd[85] = 0x02, 0x80 // 640.0 in 16.16
	tkhd[88], tkhd[89] = 0x01, 0xE0 // 480.0
	moov := append([]byte{0, 0, 0, byte(8 + len(tkhd))}, append([]byte("moov"), tkhd...)...)
	ftyp := append([]byte{0, 0, 0, 16}, []byte("ftypisom\x00\x00\x00\x00")...)
	p := filepath.Join(t.TempDir(), "a.mp4")
	_ = os.WriteFile(p, append(ftyp, moov...), 0o644)
	if w, h, err := VideoSize(p); err != nil || w != 640 || h != 480 {
		t.Fatalf("mp4: got %dx%d %v", w, h, err)
	}
}
