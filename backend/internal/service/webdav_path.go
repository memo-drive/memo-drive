package service

import (
	"path"
	"strings"
	"unicode/utf8"
)

func validateWebDAVVirtualPath(value string) error {
	if value == "" || value == "/" {
		return nil
	}
	if strings.TrimSpace(value) != value || !strings.HasPrefix(value, "/") {
		return ErrInvalidWebDAVPath
	}
	if strings.Contains(value, "\x00") || strings.Contains(value, "\\") || strings.Contains(value, "//") {
		return ErrInvalidWebDAVPath
	}
	if path.Clean(value) != value {
		return ErrInvalidWebDAVPath
	}
	segments := strings.Split(strings.TrimPrefix(value, "/"), "/")
	for i, segment := range segments {
		if !validWebDAVVirtualPathSegment(segment) {
			return ErrInvalidWebDAVPath
		}
		if i == 0 && strings.EqualFold(segment, ".trash") {
			return ErrInvalidWebDAVPath
		}
	}
	return nil
}

func validWebDAVVirtualPathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." || !utf8.ValidString(segment) {
		return false
	}
	if strings.Contains(segment, "\x00") || strings.Contains(segment, "/") || strings.Contains(segment, "\\") {
		return false
	}
	if strings.TrimSpace(segment) != segment || strings.HasSuffix(segment, ".") {
		return false
	}
	if strings.Trim(segment, ". ") == "" {
		return false
	}
	if strings.HasPrefix(segment, ".") && (len(segment) == 1 || segment[1] == '.' || segment[1] == ' ') {
		return false
	}
	return !strings.ContainsAny(segment, `:*?"<>|`)
}
