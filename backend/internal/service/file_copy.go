package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/model"
)

// FileCopyInput is the public destination contract for copying one File tree.
type FileCopyInput struct {
	Path           string             `json:"path"`
	Name           string             `json:"name"`
	ConflictPolicy FileConflictPolicy `json:"conflict_policy"`
}

type FileCopySummary struct {
	Files   int `json:"files"`
	Folders int `json:"folders"`
}

// FolderCopyLimitError reports a source tree larger than the configured boundary.
type FolderCopyLimitError struct {
	Nodes    int
	MaxNodes int
}

type FolderReplaceUnsupportedError struct{}

func (e *FolderReplaceUnsupportedError) Error() string {
	return "Folder Copy does not support replace"
}

func (e *FolderCopyLimitError) Error() string {
	return "folder copy exceeds maximum node count"
}

// FileCopyResult returns the copied root and a count of copied resources.
type FileCopyResult struct {
	File    *model.File     `json:"file"`
	Summary FileCopySummary `json:"summary"`
	TaskID  string          `json:"task_id,omitempty"`
	Created bool            `json:"-"`
}

// Copy creates an independent storage object for a File at the requested target.
func (s *FileService) Copy(ctx context.Context, sourceID string, input FileCopyInput) (*FileCopyResult, error) {
	return s.copy(ctx, sourceID, input, true)
}

// copy executes Copy with optional per-File target locking. Callers that disable it
// must already own a lock covering the full destination tree.
func (s *FileService) copy(ctx context.Context, sourceID string, input FileCopyInput, lockTargets bool) (*FileCopyResult, error) {
	source, err := s.store.GetFile(ctx, sourceID)
	if err != nil {
		return nil, err
	}
	targetPath := CleanVirtualPath(input.Path)
	targetName := SafeName(input.Name)
	if targetName == "" {
		targetName = source.Name
	}
	policy := input.ConflictPolicy
	if policy == "" {
		policy = ConflictReject
	}
	if !policy.valid() {
		return nil, &InvalidConflictPolicyError{Policy: policy}
	}
	if source.IsDir && policy == ConflictReplace {
		return nil, &FolderReplaceUnsupportedError{}
	}
	for conflictAttempt := 0; ; conflictAttempt++ {
		resolution, err := (fileConflictResolver{store: s.store}).Resolve(ctx, targetPath, targetName, policy)
		if err != nil {
			return nil, err
		}
		if source.IsDir {
			result, err := s.copyFolder(ctx, source, targetPath, resolution.Resolved, lockTargets)
			if err == nil {
				return result, nil
			}
			if policy == ConflictRename && errors.Is(err, ErrPathConflict) && conflictAttempt+1 < maxConflictRenameAttempts {
				continue
			}
			return nil, err
		}
		copyID := uuid.NewString()
		storagePath := filepath.ToSlash(s.BuildStorageRel(copyID, targetPath, resolution.Resolved))
		mutationKind := model.FileMutationKindCopyCreate
		var targetFile *model.File
		created := true
		if policy == ConflictReplace && resolution.ExistingFile != nil {
			copyID = resolution.ExistingFile.ID
			storagePath = resolution.ExistingFile.StoragePath
			mutationKind = model.FileMutationKindCopyReplace
			targetFile = resolution.ExistingFile
			created = false
		}
		copied := &model.File{
			ID:          copyID,
			Name:        resolution.Resolved,
			Path:        targetPath,
			StoragePath: storagePath,
			Size:        source.Size,
			MimeType:    source.MimeType,
			Status:      model.FileStatusUploaded,
		}
		sourcePath := s.absStoragePath(source.StoragePath)
		mutations := NewFileMutationService(s.cfg, s.store, s.pipeline)
		mutationInput := FileMutationInput{
			Kind:       mutationKind,
			File:       copied,
			TargetFile: targetFile,
		}
		if targetFile != nil {
			mutationInput.VersionSource = model.FileVersionSourceCopyReplace
		}
		writeContent := func(writer io.Writer) error {
			reader, err := os.Open(sourcePath)
			if err != nil {
				return err
			}
			defer reader.Close()
			_, err = io.Copy(writer, reader)
			return err
		}
		var mutation *FileMutationResult
		if lockTargets {
			mutation, err = mutations.Apply(ctx, mutationInput, writeContent)
		} else {
			mutation, err = mutations.applyLocked(ctx, mutationInput, writeContent)
		}
		if err != nil {
			var conflict *FileConflictError
			if policy == ConflictRename && errors.As(err, &conflict) && conflictAttempt+1 < maxConflictRenameAttempts {
				continue
			}
			return nil, err
		}
		result := &FileCopyResult{
			File:    mutation.File,
			Summary: FileCopySummary{Files: 1},
			Created: created,
		}
		if mutation.Task != nil {
			result.TaskID = mutation.Task.ID
		}
		return result, nil
	}
}

func (s *FileService) copyFolder(
	ctx context.Context,
	source *model.File,
	targetPath string,
	targetName string,
	lockTargets bool,
) (*FileCopyResult, error) {
	sourceVirtual := CleanVirtualPath(path.Join(source.Path, source.Name))
	targetVirtual := CleanVirtualPath(path.Join(targetPath, targetName))
	descendants, err := s.store.ListDescendants(ctx, sourceVirtual)
	if err != nil {
		return nil, err
	}
	maxNodes := s.cfg.Storage.FolderCopyMaxNodes
	if maxNodes <= 0 {
		maxNodes = 10000
	}
	nodes := len(descendants) + 1
	if nodes > maxNodes {
		return nil, &FolderCopyLimitError{Nodes: nodes, MaxNodes: maxNodes}
	}
	operation := &model.FileCopyOperation{
		ID:       uuid.NewString(),
		SourceID: source.ID,
		State:    model.FileCopyOperationStateRunning,
	}
	if err := s.store.CreateFileCopyOperation(ctx, operation); err != nil {
		return nil, err
	}
	root, err := s.createFolderCopyRoot(ctx, operation.ID, targetPath, targetName)
	if err != nil {
		_ = s.store.UpdateFileCopyOperationState(ctx, operation.ID, model.FileCopyOperationStateFailed, err.Error())
		return nil, err
	}
	summary := FileCopySummary{Folders: 1}
	for i := range descendants {
		child := &descendants[i]
		relativeParent := strings.TrimPrefix(child.Path, sourceVirtual)
		destinationParent := CleanVirtualPath(targetVirtual + relativeParent)
		if child.IsDir {
			if _, err := s.CreateFolder(ctx, destinationParent, child.Name); err != nil {
				return nil, s.rollbackFolderCopy(ctx, operation.ID, root.ID, err)
			}
			summary.Folders++
			continue
		}
		if _, err := s.copy(ctx, child.ID, FileCopyInput{
			Path:           destinationParent,
			Name:           child.Name,
			ConflictPolicy: ConflictReject,
		}, lockTargets); err != nil {
			return nil, s.rollbackFolderCopy(ctx, operation.ID, root.ID, err)
		}
		summary.Files++
	}
	if err := s.store.UpdateFileCopyOperationState(ctx, operation.ID, model.FileCopyOperationStateCompleted, ""); err != nil {
		return nil, s.rollbackFolderCopy(ctx, operation.ID, root.ID, err)
	}
	return &FileCopyResult{File: root, Summary: summary, Created: true}, nil
}

func (s *FileService) createFolderCopyRoot(ctx context.Context, operationID, targetPath, targetName string) (*model.File, error) {
	virtual := path.Join(targetPath, targetName)
	rel := strings.TrimPrefix(virtual, "/")
	abs := s.absStoragePath(rel)
	if err := os.Mkdir(abs, 0o755); err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("%w: target Folder storage already exists", ErrPathConflict)
		}
		return nil, err
	}
	root := &model.File{
		ID: uuid.NewString(), Name: targetName, Path: targetPath, StoragePath: filepath.ToSlash(rel),
		IsDir: true, Status: model.FileStatusReady,
	}
	if err := s.store.CreateFolderCopyRoot(ctx, operationID, root); err != nil {
		_ = os.Remove(abs)
		return nil, mapStorePathConflict(err)
	}
	return root, nil
}

func (s *FileService) rollbackFolderCopy(ctx context.Context, operationID, rootID string, cause error) error {
	if err := s.SoftDelete(ctx, rootID); err != nil {
		_ = s.store.UpdateFileCopyOperationState(ctx, operationID, model.FileCopyOperationStateFailed, cause.Error())
		return fmt.Errorf("%w; folder copy rollback soft delete failed: %v", cause, err)
	}
	if err := s.Purge(ctx, rootID); err != nil {
		_ = s.store.UpdateFileCopyOperationState(ctx, operationID, model.FileCopyOperationStateFailed, cause.Error())
		return fmt.Errorf("%w; folder copy rollback purge failed: %v", cause, err)
	}
	if err := s.store.UpdateFileCopyOperationState(ctx, operationID, model.FileCopyOperationStateFailed, cause.Error()); err != nil {
		return fmt.Errorf("%w; folder copy operation failure state: %v", cause, err)
	}
	return cause
}
