//go:build !windows

package storagefs

import "os"

// Replace atomically publishes source at destination on the same filesystem.
func Replace(source, destination string) error {
	return os.Rename(source, destination)
}

// SyncDirectory persists directory entry changes where the platform supports it.
func SyncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
