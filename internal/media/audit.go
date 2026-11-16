package media

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

var videoExt = map[string]bool{".mkv": true, ".mp4": true, ".m4v": true, ".mov": true, ".webm": true, ".avi": true, ".ts": true, ".m2ts": true, ".mpg": true, ".mpeg": true, ".wmv": true}

// IsVideo reports whether a file name has a video extension.
func IsVideo(name string) bool { return videoExt[strings.ToLower(filepath.Ext(name))] }

// File is one library video found by an audit.
type File struct {
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Changed time.Time `json:"changed"`
	Codec   string    `json:"codec"`           // "av1", "h264", ... or "unknown"
	Err     string    `json:"error,omitempty"` // why the codec could not be read
}

// Scan walks the roots and returns every video that changed at or after since and is not
// AV1, oldest first. Files whose codec cannot be read are included as "unknown", so
// nothing is silently skipped. Folders and files that cannot be read are passed over.
func Scan(roots []string, since time.Time) []File {
	var out []File
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && p != root {
					return fs.SkipDir
				}
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 || !IsVideo(d.Name()) || strings.HasPrefix(d.Name(), ".") {
				return nil
			}
			fi, err := d.Info()
			if err != nil || !fi.Mode().IsRegular() {
				return nil
			}
			ch := changed(fi)
			if ch.Before(since) {
				return nil
			}
			f := File{Path: p, Size: fi.Size(), Changed: ch}
			c, err := VideoCodec(p)
			if err != nil {
				f.Codec, f.Err = "unknown", err.Error()
			} else if c == "av1" {
				return nil
			} else {
				f.Codec = c
			}
			out = append(out, f)
			return nil
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Changed.Before(out[j].Changed) })
	return out
}
