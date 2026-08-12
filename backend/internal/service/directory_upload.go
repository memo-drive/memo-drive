package service

import (
	"context"
	"errors"
	"path"
	"strings"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/store"
)

// DirectoryTooManyEntriesError reports a directory batch beyond its admission limit.
type DirectoryTooManyEntriesError struct {
	EntryCount int
	MaxEntries int
}

func (e *DirectoryTooManyEntriesError) Error() string {
	return "directory upload contains too many entries"
}

// DirectoryPrepareInput describes the local Files selected from one directory.
type DirectoryPrepareInput struct {
	DestPath string                  `json:"dest_path"`
	Entries  []DirectoryPrepareEntry `json:"entries"`
}

// DirectoryPrepareEntry identifies one local File by its relative path.
type DirectoryPrepareEntry struct {
	ClientID     string `json:"client_id"`
	RelativePath string `json:"relative_path"`
	FileSize     int64  `json:"file_size"`
}

// DirectoryPreparedFolder reports a Folder made available for the batch.
type DirectoryPreparedFolder struct {
	RelativePath string `json:"relative_path"`
	Status       string `json:"status"`
}

// DirectoryPreparedEntry is a File target ready for the existing Upload Session API.
type DirectoryPreparedEntry struct {
	ClientID     string               `json:"client_id"`
	RelativePath string               `json:"relative_path"`
	DestPath     string               `json:"dest_path,omitempty"`
	FileName     string               `json:"file_name,omitempty"`
	Status       string               `json:"status"`
	Conflict     bool                 `json:"conflict"`
	ExistingID   string               `json:"existing_file_id,omitempty"`
	RenameName   string               `json:"rename_suggestion,omitempty"`
	Error        *DirectoryEntryError `json:"error,omitempty"`
}

// DirectoryEntryError is a structured per-File preparation error.
type DirectoryEntryError struct {
	Code      string                    `json:"code"`
	Message   string                    `json:"message"`
	Retryable bool                      `json:"retryable"`
	Details   DirectoryEntryErrorDetail `json:"details"`
}

// DirectoryEntryErrorDetail identifies the rejected path and rule.
type DirectoryEntryErrorDetail struct {
	RelativePath   string `json:"relative_path"`
	Reason         string `json:"reason"`
	ExistingFileID string `json:"existing_file_id,omitempty"`
}

// DirectoryPrepareResult reports the target of every selected File.
type DirectoryPrepareResult struct {
	BatchID string                    `json:"batch_id"`
	Folders []DirectoryPreparedFolder `json:"folders"`
	Entries []DirectoryPreparedEntry  `json:"entries"`
}

// PrepareDirectory creates missing parent Folders and returns targets for Upload Init.
func (s *UploadService) PrepareDirectory(ctx context.Context, input DirectoryPrepareInput) (*DirectoryPrepareResult, error) {
	if s.cfg.Storage.DirectoryMaxEntries > 0 && len(input.Entries) > s.cfg.Storage.DirectoryMaxEntries {
		return nil, &DirectoryTooManyEntriesError{
			EntryCount: len(input.Entries),
			MaxEntries: s.cfg.Storage.DirectoryMaxEntries,
		}
	}
	basePath := CleanVirtualPath(input.DestPath)
	if basePath != "/" {
		destination, err := s.store.GetActiveByPath(ctx, path.Dir(basePath), path.Base(basePath))
		if errors.Is(err, store.ErrNotFound) || err == nil && !destination.IsDir {
			return nil, &ParentFolderNotFoundError{Path: basePath}
		}
		if err != nil {
			return nil, err
		}
	}
	result := &DirectoryPrepareResult{
		BatchID: uuid.NewString(),
		Folders: []DirectoryPreparedFolder{},
		Entries: make([]DirectoryPreparedEntry, 0, len(input.Entries)),
	}
	reservedTargets := make(map[string]struct{}, len(input.Entries))
	preparedFolders := make(map[string]string)
	for _, entry := range input.Entries {
		if entry.FileSize <= 0 {
			result.Entries = append(result.Entries, failedDirectoryFileSizeEntry(entry, "invalid_file_size", "non_positive"))
			continue
		}
		if entry.FileSize > s.cfg.Storage.MaxFileSize {
			result.Entries = append(result.Entries, failedDirectoryFileSizeEntry(entry, "file_too_large", "max_file_size"))
			continue
		}
		parts := strings.Split(entry.RelativePath, "/")
		if reason := s.directoryRelativePathReason(entry.RelativePath, parts); reason != "" {
			result.Entries = append(result.Entries, failedDirectoryEntry(entry, reason))
			continue
		}
		fileName := parts[len(parts)-1]
		parentPath := basePath
		blocked := false
		for index, segment := range parts[:len(parts)-1] {
			folderKey := sqliteNoCaseKey(parentPath + "\x00" + segment)
			if preparedPath, ok := preparedFolders[folderKey]; ok {
				parentPath = preparedPath
				continue
			}
			resolvedSegment := segment
			existing, err := s.store.GetActiveByPath(ctx, parentPath, segment)
			if errors.Is(err, store.ErrNotFound) {
				folder, err := s.fileStore.CreateFolder(ctx, parentPath, segment)
				if err != nil {
					return nil, err
				}
				resolvedSegment = folder.Name
				result.Folders = append(result.Folders, DirectoryPreparedFolder{
					RelativePath: strings.Join(parts[:index+1], "/"),
					Status:       "created",
				})
			} else if err != nil {
				return nil, err
			} else if existing.IsDir {
				resolvedSegment = existing.Name
				result.Folders = append(result.Folders, DirectoryPreparedFolder{
					RelativePath: strings.Join(parts[:index+1], "/"),
					Status:       "existing",
				})
			} else {
				result.Entries = append(result.Entries, failedDirectoryConflictEntry(entry, existing.ID, "file_blocks_folder"))
				blocked = true
				break
			}
			parentPath = path.Join(parentPath, resolvedSegment)
			preparedFolders[folderKey] = parentPath
		}
		if blocked {
			continue
		}
		existingTarget, targetErr := s.store.GetActiveByPath(ctx, parentPath, fileName)
		if targetErr == nil && existingTarget.IsDir {
			result.Entries = append(result.Entries, failedDirectoryConflictEntry(entry, existingTarget.ID, "folder_blocks_file"))
			continue
		}
		if targetErr != nil && !errors.Is(targetErr, store.ErrNotFound) {
			return nil, targetErr
		}
		preflight, err := s.fileStore.PreflightConflicts(ctx, parentPath, []string{fileName})
		if err != nil {
			return nil, err
		}
		item := preflight[0]
		targetKey := sqliteNoCaseKey(parentPath + "\x00" + item.NormalizedName)
		if _, duplicate := reservedTargets[targetKey]; duplicate {
			result.Entries = append(result.Entries, failedDuplicateDirectoryEntry(entry))
			continue
		}
		reservedTargets[targetKey] = struct{}{}
		result.Entries = append(result.Entries, DirectoryPreparedEntry{
			ClientID:     entry.ClientID,
			RelativePath: entry.RelativePath,
			DestPath:     parentPath,
			FileName:     item.NormalizedName,
			Status:       "ready",
			Conflict:     item.Conflict,
			ExistingID:   item.ExistingFileID,
			RenameName:   item.RenameSuggestion,
		})
	}
	return result, nil
}

func failedDirectoryFileSizeEntry(entry DirectoryPrepareEntry, code, reason string) DirectoryPreparedEntry {
	return DirectoryPreparedEntry{
		ClientID:     entry.ClientID,
		RelativePath: entry.RelativePath,
		Status:       "failed",
		Error: &DirectoryEntryError{
			Code:      code,
			Message:   "File size is outside the upload limits",
			Retryable: false,
			Details: DirectoryEntryErrorDetail{
				RelativePath: entry.RelativePath,
				Reason:       reason,
			},
		},
	}
}

func (s *UploadService) directoryRelativePathReason(relativePath string, parts []string) string {
	if reason := nonPortableRelativePathReason(relativePath); reason != "" {
		return reason
	}
	if s.cfg.Storage.DirectoryMaxPathBytes > 0 && len(relativePath) > s.cfg.Storage.DirectoryMaxPathBytes {
		return "max_path_bytes"
	}
	if s.cfg.Storage.DirectoryMaxDepth > 0 && len(parts) > s.cfg.Storage.DirectoryMaxDepth {
		return "max_depth"
	}
	if reason := ambiguousDirectorySegmentReason(relativePath, parts); reason != "" {
		return reason
	}
	if directoryPathContains(parts, "..") {
		return "parent_segment"
	}
	return ""
}

func failedDuplicateDirectoryEntry(entry DirectoryPrepareEntry) DirectoryPreparedEntry {
	return DirectoryPreparedEntry{
		ClientID:     entry.ClientID,
		RelativePath: entry.RelativePath,
		Status:       "failed",
		Error: &DirectoryEntryError{
			Code:      "duplicate_relative_path",
			Message:   "directory upload contains the same target more than once",
			Retryable: false,
			Details: DirectoryEntryErrorDetail{
				RelativePath: entry.RelativePath,
				Reason:       "case_insensitive_duplicate",
			},
		},
	}
}

func failedDirectoryConflictEntry(entry DirectoryPrepareEntry, existingFileID, reason string) DirectoryPreparedEntry {
	return DirectoryPreparedEntry{
		ClientID:     entry.ClientID,
		RelativePath: entry.RelativePath,
		Status:       "failed",
		Error: &DirectoryEntryError{
			Code:      "path_conflict",
			Message:   "a target path is occupied by an incompatible resource type",
			Retryable: false,
			Details: DirectoryEntryErrorDetail{
				RelativePath:   entry.RelativePath,
				Reason:         reason,
				ExistingFileID: existingFileID,
			},
		},
	}
}

func ambiguousDirectorySegmentReason(relativePath string, parts []string) string {
	if strings.ContainsRune(relativePath, '\x00') {
		return "nul"
	}
	if directoryPathContains(parts, "") {
		return "empty_segment"
	}
	if directoryPathContains(parts, ".") {
		return "dot_segment"
	}
	return ""
}

func nonPortableRelativePathReason(relativePath string) string {
	if strings.HasPrefix(relativePath, "/") || isWindowsAbsolutePath(relativePath) {
		return "absolute_path"
	}
	if strings.Contains(relativePath, "\\") {
		return "backslash"
	}
	return ""
}

func isWindowsAbsolutePath(value string) bool {
	if len(value) < 3 || value[1] != ':' || value[2] != '/' {
		return false
	}
	first := value[0]
	return first >= 'A' && first <= 'Z' || first >= 'a' && first <= 'z'
}

func failedDirectoryEntry(entry DirectoryPrepareEntry, reason string) DirectoryPreparedEntry {
	return DirectoryPreparedEntry{
		ClientID:     entry.ClientID,
		RelativePath: entry.RelativePath,
		Status:       "failed",
		Error: &DirectoryEntryError{
			Code:      "invalid_relative_path",
			Message:   "invalid directory upload relative path",
			Retryable: false,
			Details: DirectoryEntryErrorDetail{
				RelativePath: entry.RelativePath,
				Reason:       reason,
			},
		},
	}
}

func directoryPathContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
