package pipeline_test

import (
	"context"
	"strings"
	"testing"

	"github.com/z354prance/overmynd/internal/config"
	"github.com/z354prance/overmynd/internal/job"
	"github.com/z354prance/overmynd/internal/pipeline"
)

// sizedMKV is a small Matroska file with one video track of the given frame size.
func sizedMKV(w, h int) []byte {
	be := func(n int) []byte { return []byte{byte(n >> 8), byte(n)} }
	video := rel([]byte{0xE0}, append(rel([]byte{0xB0}, be(w)), rel([]byte{0xBA}, be(h))...))
	entry := rel([]byte{0xAE}, append(append(rel([]byte{0x83}, []byte{1}), rel([]byte{0x86}, []byte("V_MPEG4/ISO/AVC"))...), video...))
	tracks := rel([]byte{0x16, 0x54, 0xAE, 0x6B}, entry)
	data := rel([]byte{0x1F, 0x43, 0xB6, 0x75}, []byte(strings.Repeat("d", 3000)))
	hdr := rel([]byte{0x1A, 0x45, 0xDF, 0xA3}, rel([]byte{0x42, 0x82}, []byte("matroska")))
	return append(hdr, rel([]byte{0x18, 0x53, 0x80, 0x67}, append(tracks, data...))...)
}

type resArr struct {
	failArr
	res   int
	err   error
	calls int
	// from is the call number (1 is the first) from which the arr reports res; earlier
	// calls get 0 ("not said").
	from int
}

func (r *resArr) GrabbedResolution(context.Context, string) (int, error) {
	r.calls++
	if r.calls < r.from {
		return 0, r.err
	}
	return r.res, r.err
}

func runQuality(t *testing.T, res int, w, h int, skip bool) (job.Job, *resArr) {
	t.Helper()
	u := newURig(t)
	ra := &resArr{res: res, from: 1}
	u.eng.ArrFor = func(config.Category) pipeline.ImportChecker { return ra }
	if skip {
		s := u.cfg.Get()
		s.SkipQualityCheck = true
		if err := u.cfg.Set(s); err != nil {
			t.Fatal(err)
		}
	}
	id := u.addNZB("Show.S01E01.1080p")
	u.tick(2)
	u.tb.finish("101", map[string]string{"Show.S01E01.1080p/Show.S01E01.1080p.mkv": string(sizedMKV(w, h))})
	u.tick(4)
	j, _ := u.st.Get(id)
	return j, ra
}

func TestReleaseSmallerThanGrabbedFailsAndTheArrIsTold(t *testing.T) {
	j, ra := runQuality(t, 1080, 640, 480, false)
	if j.State == job.Ready || j.State == job.Imported {
		t.Fatalf("a 480p file grabbed as 1080p must not be handed back, state %s (%s)", j.State, j.Reason)
	}
	var why string
	for _, h := range j.History {
		if h.To == job.Failed {
			why = h.Reason
		}
	}
	if !strings.Contains(why, "1080p") || !strings.Contains(why, "640x480") {
		t.Fatalf("the failure reason should name both sizes: %q", why)
	}
	if len(ra.told) != 1 {
		t.Fatalf("the arr must be told to blocklist it, told %d times", len(ra.told))
	}
}

func TestReleaseThatMatchesIsHandedBack(t *testing.T) {
	for name, c := range map[string][3]int{"1080p": {1080, 1920, 1080}, "letterboxed": {1080, 1920, 800}, "720": {720, 1280, 720}, "grabbed lower": {480, 640, 480}, "arr silent": {0, 640, 480}} {
		j, ra := runQuality(t, c[0], c[1], c[2], false)
		if j.State != job.Ready || len(ra.told) != 0 {
			t.Fatalf("%s: should pass, state %s (%s), told %d", name, j.State, j.Reason, len(ra.told))
		}
	}
}

func TestQualityCheckOffAndArrErrorsNeverBlock(t *testing.T) {
	if j, _ := runQuality(t, 1080, 640, 480, true); j.State != job.Ready {
		t.Fatalf("with the check off the job must pass: %s (%s)", j.State, j.Reason)
	}
}

// The check also runs on the file that is about to be handed to the arr, so a file that
// got past the first look (or was scaled down on the way) is still stopped.
func TestHandBackChecksTheFileTheArrWillImport(t *testing.T) {
	u := newURig(t)
	ra := &resArr{res: 1080, from: 2}
	u.eng.ArrFor = func(config.Category) pipeline.ImportChecker { return ra }
	id := u.addNZB("Show.S01E01.1080p")
	u.tick(2)
	u.tb.finish("101", map[string]string{"Show.S01E01.1080p/Show.S01E01.1080p.mkv": string(sizedMKV(640, 480))})
	u.tick(4)
	j, _ := u.st.Get(id)
	if j.State == job.Ready || j.State == job.Imported || len(ra.told) != 1 {
		t.Fatalf("the hand-back check must stop it: state %s (%s), arr told %d", j.State, j.Reason, len(ra.told))
	}
	if ra.calls < 2 {
		t.Fatalf("expected a second look at hand-back, got %d lookups", ra.calls)
	}
}
