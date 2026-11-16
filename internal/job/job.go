// Package job defines the per-grab job record and its state machine.
package job

import (
	"strings"
	"time"
)

type State string

const (
	Received        State = "received"
	Submitted       State = "submitted"
	Downloading     State = "downloading"
	Pulling         State = "pulling"
	Downloaded      State = "downloaded"
	QueuedTranscode State = "queued_transcode"
	Transcoding     State = "transcoding"
	Verifying       State = "verifying"
	Ready           State = "ready"
	Imported        State = "imported"
	Cleaned         State = "cleaned"
	Failed          State = "failed"
)

type Transition struct {
	At     time.Time `json:"at"`
	From   State     `json:"from"`
	To     State     `json:"to"`
	Reason string    `json:"reason,omitempty"`
}

// Job is one grab, from the arr's hand-off to cleanup. ID is the lowercase
// torrent infohash (or the NZB ID for usenet), which is also the arr's download ID.
type Job struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Source   string `json:"source"` // radarr | sonarr | lidarr | rss
	Kind     string `json:"kind"`   // torrent | usenet

	State    State   `json:"state"`
	Reason   string  `json:"reason,omitempty"`
	Progress float64 `json:"progress"` // backend download progress, 0..1
	Size     int64   `json:"size"`

	TranscodeProgress float64 `json:"transcodeProgress,omitempty"` // 0..1 from the Tdarr worker
	TranscodeNode     string  `json:"transcodeNode,omitempty"`
	TranscodeWorker   string  `json:"transcodeWorker,omitempty"`
	TranscodeSig      string  `json:"transcodeSig,omitempty"` // sizes seen last check, to tell when output stopped growing

	PullDone  int64 `json:"pullDone,omitempty"` // bytes copied from the remote backend to staging
	PullTotal int64 `json:"pullTotal,omitempty"`

	Magnet      string `json:"magnet,omitempty"`
	TorrentPath string `json:"torrentPath,omitempty"`
	NZBPath     string `json:"nzbPath,omitempty"`
	Backend     string `json:"backend,omitempty"`
	RemoteID    string `json:"remoteId,omitempty"`
	SavePath    string `json:"savePath,omitempty"`

	StagingDir  string   `json:"stagingDir,omitempty"`
	CompleteDir string   `json:"completeDir,omitempty"`
	Files       []string `json:"files,omitempty"` // staged, relative to StagingDir/CompleteDir

	Attempts int       `json:"attempts"` // retries inside the current state; reset on every transition
	Retries  int       `json:"retries"`  // re-submissions after a stall or client error; kept across states
	NextRun  time.Time `json:"nextRun,omitempty"`

	Created      time.Time `json:"created"`
	Updated      time.Time `json:"updated"`
	StateChanged time.Time `json:"stateChanged"`
	LastProgress time.Time `json:"lastProgress"`

	ArrDeleted    bool     `json:"arrDeleted,omitempty"`    // the arr called delete on this item
	PastRemoteIDs []string `json:"pastRemoteIds,omitempty"` // client items from earlier attempts, removed at cleanup
	ArrNotified   bool     `json:"arrNotified,omitempty"`   // the arr was told to blocklist the release
	FailCleaned   bool     `json:"failCleaned,omitempty"`   // files and backend item removed after a failure
	Flag          string   `json:"flag,omitempty"`          // reconciler note shown in the UI

	// Re-encode jobs (Source "reencode") work on a file already in the library.
	Original     string `json:"original,omitempty"`     // the library file being re-encoded
	OriginalSize int64  `json:"originalSize,omitempty"` // its size and modified time when it was queued,
	OriginalMod  int64  `json:"originalMod,omitempty"`  // so a file that changed meanwhile is left alone

	History []Transition `json:"history,omitempty"`
}

// Move records a transition. It never changes the state to itself.
func (j *Job) Move(to State, reason string, now time.Time) {
	if j.State == to {
		j.Reason = reason
		j.Updated = now
		return
	}
	j.History = append(j.History, Transition{At: now, From: j.State, To: to, Reason: reason})
	if len(j.History) > 200 {
		j.History = j.History[len(j.History)-200:]
	}
	j.State = to
	j.Reason = reason
	j.StateChanged = now
	j.Updated = now
	j.Attempts = 0
	j.NextRun = time.Time{}
	j.NextRun = time.Time{}
}

func (j Job) Clone() Job {
	c := j
	c.Files = append([]string(nil), j.Files...)
	c.History = append([]Transition(nil), j.History...)
	return c
}

// Active reports whether the pipeline still owns the job.
func (j Job) Active() bool { return j.State != Cleaned }

// Group is the coarse state shown on the public side.
func Group(s State) string {
	switch s {
	case Received, Submitted, QueuedTranscode:
		return "queued"
	case Downloading, Pulling, Downloaded:
		return "downloading"
	case Transcoding, Verifying:
		return "processing"
	case Ready, Imported:
		return "finishing"
	case Cleaned:
		return "done"
	case Failed:
		return "failed"
	}
	return "queued"
}

// Overall is the progress shown to the arr: the pipeline as a whole, never
// 100% until the file is in the complete folder.
func Overall(j Job) float64 {
	switch j.State {
	case Received, Submitted:
		return 0
	case Downloading:
		return j.Progress * 0.8
	case Pulling:
		if j.PullTotal > 0 {
			return 0.8 + 0.1*float64(j.PullDone)/float64(j.PullTotal)
		}
		return 0.8
	case Downloaded, QueuedTranscode:
		return 0.9
	case Transcoding:
		return 0.9 + 0.05*j.TranscodeProgress
	case Verifying:
		return 0.95
	case Ready, Imported, Cleaned:
		return 1
	}
	return j.Progress * 0.8
}

// DownloadID is the ID the arr recorded for this grab: torrent hashes are
// upper-case in the arrs' history, NZB IDs are used exactly as handed out.
func (j Job) DownloadID() string {
	if j.Kind == "torrent" {
		return strings.ToUpper(j.ID)
	}
	return j.ID
}

// FailedStage names the step a failed job was on when it failed: "download"
// (to the download client or TorBox), "transfer" (from TorBox to this server),
// "convert" (Tdarr and the check after it) or "ready" (handing back to the arr).
// It is empty for a job that has not failed.
func FailedStage(j Job) string {
	if j.State != Failed {
		return ""
	}
	from := Downloading
	for i := len(j.History) - 1; i >= 0; i-- {
		if j.History[i].To == Failed {
			from = j.History[i].From
			break
		}
	}
	switch from {
	case Pulling, Downloaded:
		return "transfer"
	case QueuedTranscode, Transcoding, Verifying:
		return "convert"
	case Ready, Imported:
		return "ready"
	}
	return "download"
}
