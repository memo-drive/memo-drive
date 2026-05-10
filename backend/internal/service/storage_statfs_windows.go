//go:build windows

package service

import (
	"math"
	"syscall"
)

func filesystemTotalBytes(root string) (int64, error) {
	path, err := syscall.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailable uint64
	var totalBytes uint64
	var totalFreeBytes uint64
	if err := syscall.GetDiskFreeSpaceEx(path, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return 0, err
	}
	if totalBytes > uint64(math.MaxInt64) {
		return math.MaxInt64, nil
	}
	return int64(totalBytes), nil
}
