//go:build !linux

package storage

import "errors"

func available(path string) (uint64, uint64, error) {
	return 0, 0, errors.New("filesystem capacity supported on Linux")
}
