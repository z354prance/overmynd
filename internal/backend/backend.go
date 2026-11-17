// Package backend defines the download backends the pipeline can use.
package backend

import (
	"context"
	"errors"

	"github.com/z354prance/overmynd/internal/job"
)

type Status string

const (
	Downloading Status = "downloading"
	Complete    Status = "complete"
	Errored     Status = "error"
	Missing     Status = "missing"
)

type Item struct {
	Status   Status
	Progress float64
	Error    string
	// Inactive: the client has the item but is not working on it (TorBox: active=false, unfinished, not queued).
	Inactive bool
	// Stuck: the client reports the download complete (100%) but never hands over files.
	Stuck bool
}

type File struct {
	ID   string // remote file ID, when the backend serves files over the network
	Path string // relative to the job's save path, or the remote path
	Size int64
}

// ErrBusy means the backend cannot take the call now (limit reached, no free
// slot, cooling down). It is a reason to wait, never a failure.
var ErrBusy = errors.New("backend busy")

// Permanent marks an error that retrying will not fix.
type Permanent struct{ Msg string }

func (p Permanent) Error() string { return p.Msg }

// Fetcher is implemented by backends whose files must be copied to local disk.
type Fetcher interface {
	// Fetch copies one remote file to dst, resuming a partial copy, and reports bytes as they arrive.
	Fetch(ctx context.Context, remoteID string, f File, dst string, progress func(n int64)) error
}

type Backend interface {
	Name() string
	// Add submits the job and returns the remote ID and the local path where its files will appear.
	Add(ctx context.Context, j job.Job) (remoteID, savePath string, err error)
	Status(ctx context.Context, remoteID string) (Item, error)
	Files(ctx context.Context, remoteID string) ([]File, error)
	// Remove deletes the remote item, with its data when deleteFiles is set.
	Remove(ctx context.Context, remoteID string, deleteFiles bool) error
}
