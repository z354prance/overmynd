package pipeline

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// openUp makes everything under root readable and writable by anyone (dirs 0777,
// files 0666), the way Unraid shares expect, so a network share can move and delete
// what Undermynd delivered whatever user the files were created by. Best effort: it
// can only change what this process owns. It returns how many entries it could not
// change and a description of the first one (path and owner).
func openUp(root string) (failed int, first string) {
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		mode := os.FileMode(0o666)
		if d.IsDir() {
			mode = 0o777
		}
		if fi, ierr := d.Info(); ierr == nil && fi.Mode().Perm() == mode {
			return nil
		}
		if cerr := os.Chmod(p, mode); cerr != nil {
			failed++
			if first == "" {
				owner := "?"
				if fi, ierr := os.Stat(p); ierr == nil {
					if st, ok := fi.Sys().(*syscall.Stat_t); ok {
						owner = itoa(int(st.Uid)) + ":" + itoa(int(st.Gid))
					}
				}
				first = p + " (owner " + owner + "): " + cerr.Error()
			}
		}
		return nil
	})
	return
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func (e *Engine) openUpLog(root string) {
	if failed, first := openUp(root); failed > 0 {
		e.Log.Printf("permissions: could not open up %d item(s) under %s, first: %s", failed, root, first)
	}
}
