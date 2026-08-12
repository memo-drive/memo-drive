package service

import (
	"sort"
	"strings"
	"sync"
)

var sharedFilePathLocks = newFilePathLocks()

type filePathLocks struct {
	mu     sync.Mutex
	cond   *sync.Cond
	active map[string]int
}

func newFilePathLocks() *filePathLocks {
	locks := &filePathLocks{active: map[string]int{}}
	locks.cond = sync.NewCond(&locks.mu)
	return locks
}

func (l *filePathLocks) lock(paths ...string) func() {
	if l == nil {
		return func() {}
	}
	paths = normalizeFileLockPaths(paths)
	l.mu.Lock()
	for l.hasConflict(paths) {
		l.cond.Wait()
	}
	for _, p := range paths {
		l.active[p]++
	}
	l.mu.Unlock()

	return func() {
		l.mu.Lock()
		for _, p := range paths {
			l.active[p]--
			if l.active[p] <= 0 {
				delete(l.active, p)
			}
		}
		l.cond.Broadcast()
		l.mu.Unlock()
	}
}

func (l *filePathLocks) hasConflict(paths []string) bool {
	for active := range l.active {
		for _, candidate := range paths {
			if fileLockPathsConflict(active, candidate) {
				return true
			}
		}
	}
	return false
}

func (s *WebDAVService) lockPaths(paths ...string) func() {
	if s == nil || s.locks == nil {
		return func() {}
	}
	return s.locks.lock(paths...)
}

func normalizeFileLockPaths(paths []string) []string {
	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(paths))
	for _, p := range paths {
		p = strings.ToLower(CleanVirtualPath(p))
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		normalized = append(normalized, p)
	}
	sort.Strings(normalized)
	return normalized
}

func fileLockPathsConflict(a, b string) bool {
	a = strings.ToLower(CleanVirtualPath(a))
	b = strings.ToLower(CleanVirtualPath(b))
	if a == "/" || b == "/" || a == b {
		return true
	}
	return strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/")
}
