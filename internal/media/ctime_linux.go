//go:build linux

package media

import (
	"os"
	"syscall"
	"time"
)

// changed is the later of the content change time and the inode change time. A file
// that is moved or linked into a folder gets a fresh inode change time even though its
// content time is old, so this is when a file arrived as well as when it was written.
func changed(fi os.FileInfo) time.Time {
	t := fi.ModTime()
	if st, ok := fi.Sys().(*syscall.Stat_t); ok {
		if c := time.Unix(int64(st.Ctim.Sec), int64(st.Ctim.Nsec)); c.After(t) {
			t = c
		}
	}
	return t
}
