//go:build windows

package storagefs

import "golang.org/x/sys/windows"

// Replace atomically publishes source at destination on the same filesystem.
func Replace(source, destination string) error {
	sourcePath, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	destinationPath, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(
		sourcePath,
		destinationPath,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
}

// SyncDirectory is a no-op because Windows does not expose directory fsync.
func SyncDirectory(string) error {
	return nil
}
