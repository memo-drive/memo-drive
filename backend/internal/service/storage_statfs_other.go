//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package service

func filesystemCapacity(root string) (FilesystemCapacity, error) {
	return FilesystemCapacity{}, nil
}
