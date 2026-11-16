//go:build !linux

package pipeline

func diskSpace(path string) (free, total uint64) { return 0, 0 }
