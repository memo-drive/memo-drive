//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package maintenance

import "os"

func tryLock(_ *os.File) error     { return nil }
func unlock(_ *os.File) error      { return nil }
func isLockContended(_ error) bool { return false }
