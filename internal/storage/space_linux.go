//go:build linux

package storage

import "syscall"

func available(path string) (uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	bs := uint64(stat.Bsize)
	return uint64(stat.Bavail) * bs, uint64(stat.Blocks) * bs, nil
}
