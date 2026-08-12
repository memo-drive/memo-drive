package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/storagefs"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

// FileMutationInput describes one File content publication.
type FileMutationInput struct {
	Kind          string
	File          *model.File
	TargetFile    *model.File
	UploadID      string
	VersionSource string
}

type FileMutationResult struct {
	File *model.File
	Task *model.Task
}

// FileMutationService hides the recoverable filesystem/SQLite commit protocol.
type FileMutationService struct {
	cfg      *config.Config
	store    *store.Store
	pipeline *PipelineService
	vectorDB vectordb.VectorStore
	locks    *filePathLocks
	capacity *CapacityService
}

func NewFileMutationService(cfg *config.Config, store *store.Store, pipelines ...*PipelineService) *FileMutationService {
	var pipeline *PipelineService
	var vectorDB vectordb.VectorStore
	if len(pipelines) > 0 {
		pipeline = pipelines[0]
	}
	if pipeline != nil {
		vectorDB = pipeline.vectorDB
	}
	return &FileMutationService{
		cfg:      cfg,
		store:    store,
		pipeline: pipeline,
		vectorDB: vectorDB,
		locks:    sharedFilePathLocks,
		capacity: NewCapacityService(cfg, store),
	}
}

// Recover rolls back uncommitted filesystem changes and finalizes committed ones.
// Each state transition is safe to repeat after another interruption.
func (s *FileMutationService) Recover(ctx context.Context) error {
	mutations, err := s.store.ListRecoverableFileMutations(ctx)
	if err != nil {
		return err
	}
	for i := range mutations {
		mutation := &mutations[i]
		switch mutation.State {
		case model.FileMutationStatePrepared, model.FileMutationStateFSApplied:
			if err := s.rollbackFilesystem(mutation); err != nil {
				return fmt.Errorf("recover mutation %s rollback: %w", mutation.ID, err)
			}
			if err := s.store.UpdateFileMutationState(
				ctx,
				mutation.ID,
				model.FileMutationStateFailed,
				"rolled back during startup recovery",
			); err != nil {
				return fmt.Errorf("recover mutation %s mark failed: %w", mutation.ID, err)
			}
		case model.FileMutationStateDBCommitted:
			if _, err := os.Stat(s.abs(mutation.FinalStoragePath)); err != nil {
				return fmt.Errorf("recover mutation %s committed content: %w", mutation.ID, err)
			}
			if err := s.store.UpdateFileMutationState(
				ctx,
				mutation.ID,
				model.FileMutationStateFinalized,
				"",
			); err != nil {
				return fmt.Errorf("recover mutation %s finalize: %w", mutation.ID, err)
			}
			if err := os.RemoveAll(filepath.Dir(s.abs(mutation.StagedPath))); err != nil {
				return fmt.Errorf("recover mutation %s cleanup: %w", mutation.ID, err)
			}
		}
	}
	return nil
}

func (s *FileMutationService) SweepTerminal(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	mutations, err := s.store.ListTerminalFileMutationsBefore(ctx, olderThan, limit)
	if err != nil {
		return 0, err
	}
	removed := 0
	for i := range mutations {
		if ctx.Err() != nil {
			return removed, ctx.Err()
		}
		mutation := &mutations[i]
		stagingDir, err := s.stagingDir(mutation.StagedPath)
		if err != nil {
			log.Printf("level=warn component=file_mutation event=terminal_cleanup_rejected mutation_id=%s staged_path=%q err=%q", mutation.ID, mutation.StagedPath, err)
			continue
		}
		if err := os.RemoveAll(stagingDir); err != nil {
			log.Printf("level=warn component=file_mutation event=terminal_cleanup_failed mutation_id=%s path=%q err=%q", mutation.ID, stagingDir, err)
			continue
		}
		if err := s.store.DeleteTerminalFileMutation(ctx, mutation.ID); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (s *FileMutationService) SweepOrphanStaging(ctx context.Context, olderThan time.Time, limit int) (int, error) {
	known, err := s.store.ListFileMutationIDs(ctx)
	if err != nil {
		return 0, err
	}
	root := s.abs(".staging")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if limit <= 0 {
		limit = 100
	}
	removed := 0
	for _, entry := range entries {
		if removed >= limit || ctx.Err() != nil {
			break
		}
		if !entry.IsDir() {
			continue
		}
		if _, ok := known[entry.Name()]; ok {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			log.Printf("level=warn component=file_mutation event=orphan_staging_stat_failed mutation_id=%s err=%q", entry.Name(), err)
			continue
		}
		if !info.ModTime().Before(olderThan) {
			continue
		}
		path := filepath.Join(root, entry.Name())
		if err := os.RemoveAll(path); err != nil {
			log.Printf("level=warn component=file_mutation event=orphan_staging_cleanup_failed mutation_id=%s path=%q err=%q", entry.Name(), path, err)
			continue
		}
		removed++
	}
	return removed, ctx.Err()
}

// Apply stages complete content, publishes it atomically, and commits File metadata.
func (s *FileMutationService) Apply(
	ctx context.Context,
	input FileMutationInput,
	writeContent func(io.Writer) error,
) (*FileMutationResult, error) {
	if input.File == nil {
		return nil, errors.New("file mutation requires File metadata")
	}
	unlock := s.locks.lock(virtualTarget(input.File.Path, input.File.Name))
	defer unlock()
	return s.applyLocked(ctx, input, writeContent)
}

// applyLocked executes a mutation while the caller owns the shared Target Path lock.
func (s *FileMutationService) applyLocked(
	ctx context.Context,
	input FileMutationInput,
	writeContent func(io.Writer) error,
) (*FileMutationResult, error) {
	if input.File == nil {
		return nil, errors.New("file mutation requires File metadata")
	}
	mutationID := uuid.NewString()
	stagingRel := filepath.ToSlash(filepath.Join(".staging", mutationID))
	stagedRel := filepath.ToSlash(filepath.Join(stagingRel, "content"))
	stagedAbs := s.abs(stagedRel)
	if err := os.MkdirAll(filepath.Dir(stagedAbs), 0o755); err != nil {
		return nil, err
	}
	if err := writeSyncedFile(stagedAbs, writeContent); err != nil {
		_ = os.RemoveAll(s.abs(stagingRel))
		return nil, err
	}
	stagedInfo, err := os.Stat(stagedAbs)
	if err != nil {
		_ = os.RemoveAll(s.abs(stagingRel))
		return nil, err
	}
	if stagedInfo.Size() != input.File.Size {
		_ = os.RemoveAll(s.abs(stagingRel))
		return nil, fmt.Errorf(
			"staged content size mismatch: expected %d bytes, got %d",
			input.File.Size,
			stagedInfo.Size(),
		)
	}
	if input.File.MimeType == "" {
		input.File.MimeType = detectMime(stagedAbs, input.File.Name)
	}
	replacedBytes := int64(0)
	if input.TargetFile != nil {
		replacedBytes = input.TargetFile.Size
	}
	versioning := s.cfg.FileVersion.Enabled && input.TargetFile != nil
	if versioning {
		replacedBytes = 0
	}
	if err := s.capacity.Check(ctx, CapacityRequest{
		LogicalBytes:         input.File.Size,
		ReplacedLogicalBytes: replacedBytes,
	}); err != nil {
		_ = os.RemoveAll(s.abs(stagingRel))
		return nil, err
	}

	mutation := &model.FileMutation{
		ID:               mutationID,
		Kind:             input.Kind,
		State:            model.FileMutationStatePrepared,
		VirtualPath:      virtualTarget(input.File.Path, input.File.Name),
		StagedPath:       stagedRel,
		FinalStoragePath: input.File.StoragePath,
	}
	if input.TargetFile != nil {
		mutation.TargetFileID = input.TargetFile.ID
		if versioning {
			versionID := uuid.NewString()
			mutation.OldStoragePath = filepath.ToSlash(filepath.Join(".versions", input.TargetFile.ID, versionID))
		} else {
			mutation.OldStoragePath = filepath.ToSlash(filepath.Join(stagingRel, "rollback"))
		}
	}
	if err := s.store.CreateFileMutation(ctx, mutation); err != nil {
		_ = os.RemoveAll(s.abs(stagingRel))
		return nil, err
	}

	if err := s.applyFilesystem(mutation, input.TargetFile); err != nil {
		_ = s.rollbackFilesystem(mutation)
		_ = s.store.UpdateFileMutationState(ctx, mutation.ID, model.FileMutationStateFailed, err.Error())
		return nil, err
	}
	if err := s.store.UpdateFileMutationState(ctx, mutation.ID, model.FileMutationStateFSApplied, ""); err != nil {
		_ = s.rollbackFilesystem(mutation)
		_ = s.store.UpdateFileMutationState(ctx, mutation.ID, model.FileMutationStateFailed, err.Error())
		return nil, err
	}
	mutation.State = model.FileMutationStateFSApplied

	var version *model.FileVersion
	if versioning {
		checksum, err := sha256File(s.abs(mutation.OldStoragePath))
		if err != nil {
			_ = s.rollbackFilesystem(mutation)
			_ = s.store.UpdateFileMutationState(ctx, mutation.ID, model.FileMutationStateFailed, err.Error())
			return nil, err
		}
		source := input.VersionSource
		if source == "" {
			source = model.FileVersionSourceUploadReplace
		}
		version = &model.FileVersion{
			ID:          filepath.Base(mutation.OldStoragePath),
			FileID:      input.TargetFile.ID,
			StoragePath: mutation.OldStoragePath,
			Size:        input.TargetFile.Size,
			MimeType:    input.TargetFile.MimeType,
			SHA256:      checksum,
			Source:      source,
		}
	}

	var task *model.Task
	if s.pipeline != nil {
		task = newPipelineTask(input.File.ID)
	}
	if err := s.store.CommitFileMutation(ctx, mutation, input.File, input.UploadID, task, version); err != nil {
		rollbackErr := s.rollbackFilesystem(mutation)
		errorText := err.Error()
		if rollbackErr != nil {
			errorText = fmt.Sprintf("%s; rollback: %v", errorText, rollbackErr)
		}
		_ = s.store.UpdateFileMutationState(ctx, mutation.ID, model.FileMutationStateFailed, errorText)
		if errors.Is(err, store.ErrPathConflict) {
			return nil, &FileConflictError{
				Path:           input.File.Path,
				Name:           input.File.Name,
				ExistingFileID: mutation.TargetFileID,
			}
		}
		return nil, err
	}

	if err := s.store.UpdateFileMutationState(ctx, mutation.ID, model.FileMutationStateFinalized, ""); err != nil {
		log.Printf("level=warn component=file_mutation event=finalize_state_failed mutation_id=%s err=%q", mutation.ID, err)
	} else if err := os.RemoveAll(s.abs(stagingRel)); err != nil {
		log.Printf("level=warn component=file_mutation event=staging_cleanup_failed mutation_id=%s path=%q err=%q", mutation.ID, stagingRel, err)
	}
	file, err := s.store.GetFile(ctx, input.File.ID)
	if err != nil {
		return nil, err
	}
	s.cleanupPreviousVectorIndex(ctx, input.TargetFile)
	if task != nil {
		if err := s.pipeline.submitPersisted(task, file); err != nil {
			log.Printf("level=warn component=file_mutation event=pipeline_enqueue_failed task_id=%s file_id=%s name=%q err=%q", task.ID, file.ID, file.Name, err)
			if errors.Is(err, ErrPipelineQueueFull) {
				s.pipeline.failTask(ctx, task.ID, file.ID, err)
				errText := err.Error()
				task.Status = model.TaskStatusFailed
				task.Progress = pipelineProgressCompleted
				task.Error = &errText
				file.Status = model.FileStatusFailed
			}
		}
	}
	return &FileMutationResult{File: file, Task: task}, nil
}

func (s *FileMutationService) cleanupPreviousVectorIndex(ctx context.Context, file *model.File) {
	if file == nil || file.IsDir || file.ChunkCount <= 0 || s.vectorDB == nil {
		return
	}
	ids := indexing.ChunkIDs(file.ID, file.ChunkCount)
	if err := s.vectorDB.Delete(ctx, vectordb.DefaultCollection, ids); err != nil {
		log.Printf("level=warn component=file_mutation event=vector_cleanup_failed file_id=%s chunk_count=%d err=%q", file.ID, file.ChunkCount, err)
	}
}

func (s *FileMutationService) applyFilesystem(mutation *model.FileMutation, target *model.File) error {
	finalAbs := s.abs(mutation.FinalStoragePath)
	if err := os.MkdirAll(filepath.Dir(finalAbs), 0o755); err != nil {
		return err
	}
	if target != nil {
		oldAbs := s.abs(target.StoragePath)
		rollbackAbs := s.abs(mutation.OldStoragePath)
		if err := os.MkdirAll(filepath.Dir(rollbackAbs), 0o755); err != nil {
			return err
		}
		if err := preserveRollbackFile(oldAbs, rollbackAbs); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := storagefs.Replace(s.abs(mutation.StagedPath), finalAbs); err != nil {
		return err
	}
	return storagefs.SyncDirectory(filepath.Dir(finalAbs))
}

func (s *FileMutationService) rollbackFilesystem(mutation *model.FileMutation) error {
	finalAbs := s.abs(mutation.FinalStoragePath)
	if mutation.TargetFileID == "" {
		if err := os.Remove(finalAbs); err != nil && !os.IsNotExist(err) {
			return err
		}
		return storagefs.SyncDirectory(filepath.Dir(finalAbs))
	}
	rollbackAbs := s.abs(mutation.OldStoragePath)
	if _, err := os.Stat(rollbackAbs); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := storagefs.Replace(rollbackAbs, finalAbs); err != nil {
		return err
	}
	return storagefs.SyncDirectory(filepath.Dir(finalAbs))
}

func (s *FileMutationService) abs(rel string) string {
	return filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(rel))
}

func (s *FileMutationService) stagingDir(stagedPath string) (string, error) {
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(stagedPath)))
	if !strings.HasPrefix(cleaned, ".staging/") {
		return "", errors.New("file mutation staged path is outside .staging")
	}
	parts := strings.Split(cleaned, "/")
	if len(parts) < 3 || parts[1] == "" {
		return "", errors.New("file mutation staged path has no mutation directory")
	}
	return s.abs(filepath.ToSlash(filepath.Join(parts[0], parts[1]))), nil
}

func writeSyncedFile(path string, writeContent func(io.Writer) error) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	cleanup := true
	defer func() {
		_ = file.Close()
		if cleanup {
			_ = os.Remove(path)
		}
	}()
	if err := writeContent(file); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := storagefs.SyncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func preserveRollbackFile(source, destination string) error {
	if err := os.Link(source, destination); err == nil {
		return storagefs.SyncDirectory(filepath.Dir(destination))
	}
	sourceFile, err := os.Open(source)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	return writeSyncedFile(destination, func(writer io.Writer) error {
		_, err := io.Copy(writer, sourceFile)
		return err
	})
}

func virtualTarget(parentPath, name string) string {
	if parentPath == "/" {
		return "/" + name
	}
	return parentPath + "/" + name
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
