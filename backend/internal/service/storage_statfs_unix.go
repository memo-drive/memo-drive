//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package service

import (
	"math"
	"syscall"
)

func filesystemCapacity(root string) (FilesystemCapacity, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(root, &stat); err != nil {
		return FilesystemCapacity{}, err
	}
	return FilesystemCapacity{
		TotalBytes:     saturatedFilesystemBytes(uint64(stat.Blocks), uint64(stat.Bsize)),
		AvailableBytes: saturatedFilesystemBytes(uint64(stat.Bavail), uint64(stat.Bsize)),
	}, nil
}

func saturatedFilesystemBytes(blocks, blockSize uint64) int64 {
	if blockSize > 0 && blocks > uint64(math.MaxInt64)/blockSize {
		return math.MaxInt64
	}
	return int64(blocks * blockSize)
}
