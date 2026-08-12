//go:build windows

package service

import (
	"math"

	"golang.org/x/sys/windows"
)

func filesystemCapacity(root string) (FilesystemCapacity, error) {
	path, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return FilesystemCapacity{}, err
	}
	var freeBytesAvailable uint64
	var totalBytes uint64
	var totalFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(path, &freeBytesAvailable, &totalBytes, &totalFreeBytes); err != nil {
		return FilesystemCapacity{}, err
	}
	return FilesystemCapacity{
		TotalBytes:     saturatedWindowsBytes(totalBytes),
		AvailableBytes: saturatedWindowsBytes(freeBytesAvailable),
	}, nil
}

func saturatedWindowsBytes(bytes uint64) int64 {
	if bytes > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(bytes)
}
