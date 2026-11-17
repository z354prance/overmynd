//go:build !linux

package media

import (
	"os"
	"time"
)

func changed(fi os.FileInfo) time.Time { return fi.ModTime() }
