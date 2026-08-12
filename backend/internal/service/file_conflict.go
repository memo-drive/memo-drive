package service

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

// FileConflictPolicy defines how a File write handles an occupied Target Path.
type FileConflictPolicy string

const (
	// ConflictReject refuses to create an Upload Session when the Target Path exists.
	ConflictReject FileConflictPolicy = "reject"
	// ConflictRename keeps both Files by selecting the first available numbered name.
	ConflictRename FileConflictPolicy = "rename"
	// ConflictReplace targets the existing File without changing its logical name.
	ConflictReplace           FileConflictPolicy = "replace"
	maxConflictRenameAttempts                    = 10000
)

// InvalidConflictPolicyError reports a policy outside the public conflict enum.
type InvalidConflictPolicyError struct {
	Policy FileConflictPolicy
}

// NameExhaustedError reports a rename request with no candidate inside the bounded search.
type NameExhaustedError struct {
	Path        string
	Name        string
	MaxAttempts int
}

func (e *NameExhaustedError) Error() string {
	return "no available rename target"
}

func (e *InvalidConflictPolicyError) Error() string {
	return "overwrite_policy must be reject, rename, or replace"
}

func (p FileConflictPolicy) valid() bool {
	switch p {
	case ConflictReject, ConflictRename, ConflictReplace:
		return true
	default:
		return false
	}
}

// ParentFolderNotFoundError reports a Target Path whose parent is not an active Folder.
type ParentFolderNotFoundError struct {
	Path string
}

func (e *ParentFolderNotFoundError) Error() string {
	return "parent folder does not exist"
}

// InvalidFilePathError reports a requested Target Path that cannot name a File.
type InvalidFilePathError struct {
	Path string
	Name string
}

func (e *InvalidFilePathError) Error() string {
	return "invalid file name"
}

// FileTooLargeError reports the configured single-File size boundary.
type FileTooLargeError struct {
	FileSize    int64
	MaxFileSize int64
}

func (e *FileTooLargeError) Error() string {
	return "file exceeds maximum size"
}

func (e *FileTooLargeError) Unwrap() error {
	return ErrFileTooLarge
}

// UploadIncompleteError reports a session whose expected chunks are not all present.
type UploadIncompleteError struct {
	UploadedChunks int
	ExpectedChunks int
}

func (e *UploadIncompleteError) Error() string {
	return "upload chunks are incomplete"
}

// UploadStateConflictError reports an operation disallowed by the session state machine.
type UploadStateConflictError struct {
	Status    string
	Operation string
	Reason    string
}

func (e *UploadStateConflictError) Error() string {
	if e.Reason != "" {
		return e.Reason
	}
	return "upload session state does not allow " + e.Operation
}

// ConflictResolution is the stable result of resolving one requested Target Path.
type ConflictResolution struct {
	Policy       FileConflictPolicy
	Requested    string
	Resolved     string
	ExistingFile *model.File
}

// FileConflictPreflightItem reports one requested name without reserving it.
type FileConflictPreflightItem struct {
	RequestedName    string `json:"requested_name"`
	NormalizedName   string `json:"normalized_name"`
	Conflict         bool   `json:"conflict"`
	ExistingFileID   string `json:"existing_file_id,omitempty"`
	RenameSuggestion string `json:"rename_suggestion,omitempty"`
	ReplaceAllowed   bool   `json:"replace_allowed"`
}

// FileConflictError describes an occupied Target Path through the File write API.
type FileConflictError struct {
	Path           string
	Name           string
	ExistingFileID string
}

func (e *FileConflictError) Error() string {
	return "target already exists"
}

func (e *FileConflictError) Unwrap() error {
	return ErrPathConflict
}

type fileConflictResolver struct {
	store *store.Store
}

func (r fileConflictResolver) Resolve(
	ctx context.Context,
	parentPath string,
	requestedName string,
	policy FileConflictPolicy,
) (*ConflictResolution, error) {
	if parentPath != "/" {
		parent, err := r.store.GetActiveByPath(ctx, path.Dir(parentPath), path.Base(parentPath))
		if errors.Is(err, store.ErrNotFound) || err == nil && !parent.IsDir {
			return nil, &ParentFolderNotFoundError{Path: parentPath}
		}
		if err != nil {
			return nil, err
		}
	}
	resolution := &ConflictResolution{
		Policy:    policy,
		Requested: requestedName,
		Resolved:  requestedName,
	}
	existing, err := r.store.GetActiveByPath(ctx, parentPath, requestedName)
	if errors.Is(err, store.ErrNotFound) {
		return resolution, nil
	}
	if err != nil {
		return nil, err
	}
	resolution.ExistingFile = existing

	if policy == ConflictReject {
		return nil, &FileConflictError{
			Path:           parentPath,
			Name:           requestedName,
			ExistingFileID: existing.ID,
		}
	}
	if policy == ConflictReplace {
		if existing.IsDir {
			return nil, &FileConflictError{
				Path:           parentPath,
				Name:           requestedName,
				ExistingFileID: existing.ID,
			}
		}
		return resolution, nil
	}

	for sequence := 1; sequence <= maxConflictRenameAttempts; sequence++ {
		candidate := numberedConflictName(requestedName, sequence)
		_, err := r.store.GetActiveByPath(ctx, parentPath, candidate)
		if errors.Is(err, store.ErrNotFound) {
			resolution.Resolved = candidate
			return resolution, nil
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, &NameExhaustedError{
		Path:        parentPath,
		Name:        requestedName,
		MaxAttempts: maxConflictRenameAttempts,
	}
}

func numberedConflictName(name string, sequence int) string {
	extension := path.Ext(name)
	if strings.HasPrefix(name, ".") && strings.Count(name, ".") == 1 {
		extension = ""
	}
	base := strings.TrimSuffix(name, extension)
	return fmt.Sprintf("%s (%d)%s", base, sequence, extension)
}

func (r fileConflictResolver) Preflight(
	ctx context.Context,
	parentPath string,
	names []string,
) ([]FileConflictPreflightItem, error) {
	items := make([]FileConflictPreflightItem, 0, len(names))
	reservedNames := make(map[string]struct{}, len(names))
	for _, requestedName := range names {
		normalizedName := SafeName(requestedName)
		if normalizedName == "" {
			return nil, &InvalidFilePathError{
				Path: parentPath,
				Name: requestedName,
			}
		}
		resolution, err := r.Resolve(ctx, parentPath, normalizedName, ConflictRename)
		if err != nil {
			return nil, err
		}
		item := FileConflictPreflightItem{
			RequestedName:  requestedName,
			NormalizedName: normalizedName,
		}
		_, batchConflict := reservedNames[sqliteNoCaseKey(normalizedName)]
		if resolution.ExistingFile != nil || batchConflict {
			item.Conflict = true
			if resolution.ExistingFile != nil {
				item.ExistingFileID = resolution.ExistingFile.ID
				item.ReplaceAllowed = !resolution.ExistingFile.IsDir
			}
			for sequence := 1; sequence <= maxConflictRenameAttempts; sequence++ {
				candidate := numberedConflictName(normalizedName, sequence)
				if _, reserved := reservedNames[sqliteNoCaseKey(candidate)]; reserved {
					continue
				}
				_, err := r.store.GetActiveByPath(ctx, parentPath, candidate)
				if errors.Is(err, store.ErrNotFound) {
					item.RenameSuggestion = candidate
					break
				}
				if err != nil {
					return nil, err
				}
			}
			if item.RenameSuggestion == "" {
				return nil, &NameExhaustedError{
					Path:        parentPath,
					Name:        normalizedName,
					MaxAttempts: maxConflictRenameAttempts,
				}
			}
			reservedNames[sqliteNoCaseKey(item.RenameSuggestion)] = struct{}{}
		}
		reservedNames[sqliteNoCaseKey(normalizedName)] = struct{}{}
		items = append(items, item)
	}
	return items, nil
}

func sqliteNoCaseKey(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}
