//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package service

func filesystemTotalBytes(root string) (int64, error) {
	return 0, nil
}
