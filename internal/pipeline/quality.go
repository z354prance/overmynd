package pipeline

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/z354prance/undermynd/internal/job"
	"github.com/z354prance/undermynd/internal/media"
)

// ResolutionLooker is implemented by an arr client that knows the quality a download was
// grabbed as.
type ResolutionLooker interface {
	GrabbedResolution(ctx context.Context, downloadID string) (int, error)
}

// qualityMismatch compares the real video size of the staged files with the resolution the
// arr grabbed the release as. It returns a reason when a video is clearly smaller (a "1080p"
// release that is really 480p), and "" when everything matches or cannot be judged: the
// arr does not say, cannot be reached, or the file's size cannot be read. A check that
// cannot run never holds a download up.
func (e *Engine) qualityMismatch(ctx context.Context, j job.Job) string {
	if e.Cfg.Get().SkipQualityCheck || j.Source == "reencode" || j.Source == "rss" || j.Source == "watch" {
		return ""
	}
	cat, _ := e.Cfg.Get().Category(j.Category)
	l, ok := e.arrFor(cat).(ResolutionLooker)
	if !ok {
		return ""
	}
	cctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	want, err := l.GrabbedResolution(cctx, j.DownloadID())
	if err != nil {
		e.Log.Printf("job %s: quality check skipped, the arr could not say what it grabbed: %v", short(j.ID), err)
		return ""
	}
	want = media.ResolutionTier(want)
	if want == 0 {
		return ""
	}
	for _, f := range j.Files {
		if !isVideo(f) {
			continue
		}
		w, h, err := media.VideoSize(filepath.Join(j.StagingDir, f))
		if err != nil {
			continue
		}
		if got := media.Tier(w, h); got < want {
			return fmt.Sprintf("the arr grabbed this as %dp but the video is %dx%d (%dp)", want, w, h, got)
		}
	}
	return ""
}
