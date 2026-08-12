package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

// UploadService manages chunked file upload sessions and chunk assembly.
type UploadService struct {
	cfg       *config.Config
	store     *store.Store
	fileStore *FileService
	pipeline  *PipelineService
	mutations *FileMutationService
	capacity  *CapacityService
}

// UploadCompletion holds the result of a completed upload: the created file and its pipeline task.
type UploadCompletion struct {
	File *model.File
	Task *model.Task
}

// NewUploadService creates a new UploadService.
func NewUploadService(cfg *config.Config, store *store.Store, fileStore *FileService, pipeline *PipelineService) *UploadService {
	if fileStore != nil {
		fileStore.SetPipeline(pipeline)
	}
	return &UploadService{
		cfg:       cfg,
		store:     store,
		fileStore: fileStore,
		pipeline:  pipeline,
		mutations: NewFileMutationService(cfg, store, pipeline),
		capacity:  NewCapacityService(cfg, store),
	}
}

// InitUploadInput is the payload for initializing a new upload session.
type InitUploadInput struct {
	FileName        string             `json:"file_name"`
	FileSize        int64              `json:"file_size"`
	DestPath        string             `json:"dest_path"`
	OverwritePolicy FileConflictPolicy `json:"overwrite_policy"`
}

func (s *UploadService) Init(ctx context.Context, input InitUploadInput) (*model.UploadSession, error) {
	started := time.Now()
	if input.FileSize <= 0 {
		return nil, errors.New("file_size must be greater than zero")
	}
	if input.FileSize > s.cfg.Storage.MaxFileSize {
		return nil, &FileTooLargeError{
			FileSize:    input.FileSize,
			MaxFileSize: s.cfg.Storage.MaxFileSize,
		}
	}
	destPath := CleanVirtualPath(input.DestPath)
	name := SafeName(input.FileName)
	if name == "" {
		return nil, &InvalidFilePathError{Path: destPath, Name: input.FileName}
	}
	if input.OverwritePolicy == "" {
		input.OverwritePolicy = ConflictReject
	}
	if !input.OverwritePolicy.valid() {
		return nil, &InvalidConflictPolicyError{Policy: input.OverwritePolicy}
	}
	resolution, err := (fileConflictResolver{store: s.store}).Resolve(
		ctx,
		destPath,
		name,
		input.OverwritePolicy,
	)
	if err != nil {
		return nil, err
	}
	replacedBytes := int64(0)
	if resolution.ExistingFile != nil {
		replacedBytes = resolution.ExistingFile.Size
	}
	if err := s.capacity.Check(ctx, CapacityRequest{
		LogicalBytes:         input.FileSize,
		ReplacedLogicalBytes: replacedBytes,
		PhysicalNeedBytes:    input.FileSize,
		TempNeedBytes:        input.FileSize,
	}); err != nil {
		return nil, err
	}

	session := &model.UploadSession{
		ID:              uuid.NewString(),
		FileName:        resolution.Requested,
		RequestedName:   resolution.Requested,
		ResolvedName:    resolution.Resolved,
		OverwritePolicy: string(resolution.Policy),
		FileSize:        input.FileSize,
		ChunkSize:       s.cfg.Storage.ChunkSize,
		UploadedChunks:  []int{},
		DestPath:        destPath,
		Status:          model.UploadStatusUploading,
		ExpiresAt:       time.Now().UTC().Add(s.cfg.Storage.UploadTTL),
	}
	if resolution.ExistingFile != nil {
		session.ExistingFileID = resolution.ExistingFile.ID
	}
	if err := os.MkdirAll(s.sessionDir(session.ID), 0o755); err != nil {
		log.Printf("level=error component=upload event=init_mkdir_failed upload_id=%s file_name=%q dest_path=%q err=%q", session.ID, name, session.DestPath, err)
		return nil, err
	}
	if err := s.store.CreateUploadSession(ctx, session); err != nil {
		log.Printf("level=error component=upload event=init_store_failed upload_id=%s file_name=%q dest_path=%q err=%q", session.ID, name, session.DestPath, err)
		return nil, err
	}
	expectedChunks := int((session.FileSize + session.ChunkSize - 1) / session.ChunkSize)
	log.Printf("level=info component=upload event=init_complete upload_id=%s file_name=%q file_size=%d chunk_size=%d expected_chunks=%d dest_path=%q expires_at=%s duration_ms=%d",
		session.ID, session.FileName, session.FileSize, session.ChunkSize, expectedChunks, session.DestPath, session.ExpiresAt.Format(time.RFC3339), time.Since(started).Milliseconds())
	return session, nil
}

func (s *UploadService) GetSession(ctx context.Context, id string) (*model.UploadSession, error) {
	session, err := s.store.GetUploadSession(ctx, id)
	if err != nil {
		return nil, err
	}
	if uploadStatusCanExpire(session.Status) && time.Now().UTC().After(session.ExpiresAt) {
		if err := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusExpired); err != nil {
			log.Printf("level=warn component=upload event=session_expire_mark_failed upload_id=%s err=%q", id, err)
			return nil, err
		}
		session.Status = model.UploadStatusExpired
		log.Printf("level=info component=upload event=session_expired upload_id=%s expires_at=%s", id, session.ExpiresAt.Format(time.RFC3339))
	}
	return session, nil
}

func (s *UploadService) ListSessions(ctx context.Context, limit int) ([]model.UploadSession, error) {
	sessions, err := s.store.ListUploadSessions(ctx, limit)
	if err != nil {
		log.Printf("level=error component=upload event=list_sessions_failed err=%q", err)
		return nil, err
	}
	now := time.Now().UTC()
	for i := range sessions {
		if uploadStatusCanExpire(sessions[i].Status) && now.After(sessions[i].ExpiresAt) {
			if err := s.store.UpdateUploadStatus(ctx, sessions[i].ID, model.UploadStatusExpired); err != nil {
				log.Printf("level=warn component=upload event=list_session_expire_mark_failed upload_id=%s err=%q", sessions[i].ID, err)
				continue
			}
			sessions[i].Status = model.UploadStatusExpired
		}
	}
	log.Printf("level=info component=upload event=list_sessions_complete count=%d limit=%d", len(sessions), limit)
	return sessions, nil
}

func (s *UploadService) CancelSession(ctx context.Context, id string) error {
	started := time.Now()
	session, err := s.GetSession(ctx, id)
	if err != nil {
		log.Printf("level=warn component=upload event=cancel_lookup_failed upload_id=%s err=%q", id, err)
		return err
	}
	if !canCancelUploadStatus(session.Status) {
		log.Printf("level=warn component=upload event=cancel_rejected upload_id=%s status=%q", id, session.Status)
		return &UploadStateConflictError{
			Status:    session.Status,
			Operation: "cancel",
		}
	}
	if err := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusCancelled); err != nil {
		log.Printf("level=error component=upload event=cancel_status_failed upload_id=%s err=%q", id, err)
		return err
	}
	if err := os.RemoveAll(s.sessionDir(id)); err != nil {
		log.Printf("level=warn component=upload event=cancel_cleanup_failed upload_id=%s err=%q", id, err)
		return err
	}
	log.Printf("level=info component=upload event=cancel_complete upload_id=%s file_name=%q uploaded_chunks=%d duration_ms=%d",
		id, session.FileName, len(session.UploadedChunks), time.Since(started).Milliseconds())
	return nil
}

func (s *UploadService) DeleteSession(ctx context.Context, id string) error {
	started := time.Now()
	session, err := s.GetSession(ctx, id)
	if err != nil {
		log.Printf("level=warn component=upload event=delete_session_lookup_failed upload_id=%s err=%q", id, err)
		return err
	}
	if !canDeleteUploadStatus(session.Status) {
		log.Printf("level=warn component=upload event=delete_session_rejected upload_id=%s status=%q", id, session.Status)
		return &UploadStateConflictError{
			Status:    session.Status,
			Operation: "delete",
		}
	}
	if err := s.store.DeleteUploadSession(ctx, id); err != nil {
		log.Printf("level=error component=upload event=delete_session_failed upload_id=%s err=%q", id, err)
		return err
	}
	_ = os.RemoveAll(s.sessionDir(id))
	log.Printf("level=info component=upload event=delete_session_complete upload_id=%s status=%q duration_ms=%d", id, session.Status, time.Since(started).Milliseconds())
	return nil
}

func (s *UploadService) ClearSessions(ctx context.Context) (int, error) {
	started := time.Now()
	ids, err := s.store.ClearUploadSessions(ctx)
	if err != nil {
		log.Printf("level=error component=upload event=clear_sessions_failed err=%q", err)
		return 0, err
	}
	for _, id := range ids {
		_ = os.RemoveAll(s.sessionDir(id))
	}
	log.Printf("level=info component=upload event=clear_sessions_complete count=%d duration_ms=%d", len(ids), time.Since(started).Milliseconds())
	return len(ids), nil
}

func (s *UploadService) SaveChunk(ctx context.Context, id string, chunkIndex int, body []byte) (*model.UploadSession, error) {
	started := time.Now()
	session, err := s.GetSession(ctx, id)
	if err != nil {
		log.Printf("level=error component=upload event=save_chunk_lookup_failed upload_id=%s chunk_index=%d err=%q", id, chunkIndex, err)
		return nil, err
	}
	if !canReceiveUploadChunk(session.Status) {
		log.Printf("level=warn component=upload event=save_chunk_rejected upload_id=%s chunk_index=%d status=%q", id, chunkIndex, session.Status)
		return nil, &UploadStateConflictError{
			Status:    session.Status,
			Operation: "upload_chunk",
		}
	}
	if time.Now().UTC().After(session.ExpiresAt) {
		_ = s.store.UpdateUploadStatus(ctx, id, model.UploadStatusExpired)
		log.Printf("level=warn component=upload event=save_chunk_expired upload_id=%s chunk_index=%d expires_at=%s", id, chunkIndex, session.ExpiresAt.Format(time.RFC3339))
		return nil, errors.New("upload session expired")
	}
	if chunkIndex < 0 || int64(chunkIndex)*session.ChunkSize >= session.FileSize {
		log.Printf("level=warn component=upload event=save_chunk_invalid_index upload_id=%s chunk_index=%d file_size=%d chunk_size=%d", id, chunkIndex, session.FileSize, session.ChunkSize)
		return nil, errors.New("invalid chunk index")
	}
	expectedChunkSize := session.ChunkSize
	if remaining := session.FileSize - int64(chunkIndex)*session.ChunkSize; remaining < expectedChunkSize {
		expectedChunkSize = remaining
	}
	if int64(len(body)) > expectedChunkSize {
		log.Printf("level=warn component=upload event=save_chunk_too_large upload_id=%s chunk_index=%d bytes=%d expected_max=%d file_size=%d chunk_size=%d",
			id, chunkIndex, len(body), expectedChunkSize, session.FileSize, session.ChunkSize)
		return nil, errors.New("chunk exceeds expected size")
	}
	existingChunkBytes := int64(0)
	if info, statErr := os.Stat(s.chunkPath(id, chunkIndex)); statErr == nil {
		existingChunkBytes = info.Size()
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, statErr
	}
	tempGrowth := nonNegative(int64(len(body)) - existingChunkBytes)
	if err := s.capacity.Check(ctx, CapacityRequest{
		PhysicalNeedBytes: tempGrowth,
		TempNeedBytes:     tempGrowth,
	}); err != nil {
		return nil, err
	}

	if err := os.MkdirAll(s.sessionDir(id), 0o755); err != nil {
		log.Printf("level=error component=upload event=save_chunk_mkdir_failed upload_id=%s chunk_index=%d err=%q", id, chunkIndex, err)
		return nil, err
	}
	if err := os.WriteFile(s.chunkPath(id, chunkIndex), body, 0o644); err != nil {
		log.Printf("level=error component=upload event=save_chunk_write_failed upload_id=%s chunk_index=%d bytes=%d err=%q", id, chunkIndex, len(body), err)
		return nil, err
	}
	wasNewChunk := false
	if !containsInt(session.UploadedChunks, chunkIndex) {
		wasNewChunk = true
		session.UploadedChunks = append(session.UploadedChunks, chunkIndex)
		sort.Ints(session.UploadedChunks)
		if err := s.store.UpdateUploadChunks(ctx, id, session.UploadedChunks); err != nil {
			log.Printf("level=error component=upload event=save_chunk_store_failed upload_id=%s chunk_index=%d err=%q", id, chunkIndex, err)
			return nil, err
		}
	}
	expectedChunks := int((session.FileSize + session.ChunkSize - 1) / session.ChunkSize)
	log.Printf("level=info component=upload event=save_chunk_complete upload_id=%s chunk_index=%d bytes=%d uploaded_chunks=%d expected_chunks=%d new_chunk=%t duration_ms=%d",
		id, chunkIndex, len(body), len(session.UploadedChunks), expectedChunks, wasNewChunk, time.Since(started).Milliseconds())
	return session, nil
}

func (s *UploadService) Complete(ctx context.Context, id string) (*UploadCompletion, error) {
	if s.pipeline == nil {
		return nil, fmt.Errorf("%w: pipeline is not configured", ErrServiceUnavailable)
	}
	result, err := s.completeFile(ctx, id)
	if err != nil {
		return nil, err
	}
	return &UploadCompletion{File: result.File, Task: result.Task}, nil
}

func (s *UploadService) completeFile(ctx context.Context, id string) (*FileMutationResult, error) {
	started := time.Now()
	session, err := s.GetSession(ctx, id)
	if err != nil {
		log.Printf("level=error component=upload event=complete_lookup_failed upload_id=%s err=%q", id, err)
		return nil, err
	}
	if !canCompleteUploadStatus(session.Status) {
		log.Printf("level=warn component=upload event=complete_rejected upload_id=%s status=%q", id, session.Status)
		return nil, &UploadStateConflictError{
			Status:    session.Status,
			Operation: "complete",
		}
	}
	policy := FileConflictPolicy(session.OverwritePolicy)
	if policy == "" {
		policy = ConflictReject
	}
	if !policy.valid() {
		return nil, &InvalidConflictPolicyError{Policy: policy}
	}
	resolution, err := (fileConflictResolver{store: s.store}).Resolve(
		ctx,
		session.DestPath,
		session.FileName,
		policy,
	)
	if err != nil {
		return nil, err
	}
	existingFileID := ""
	if resolution.ExistingFile != nil {
		existingFileID = resolution.ExistingFile.ID
	}
	if err := s.store.UpdateUploadResolution(ctx, session.ID, resolution.Resolved, existingFileID); err != nil {
		return nil, err
	}
	targetName := resolution.Resolved

	expectedChunks := int((session.FileSize + session.ChunkSize - 1) / session.ChunkSize)
	if len(session.UploadedChunks) != expectedChunks {
		log.Printf("level=warn component=upload event=complete_missing_chunks upload_id=%s uploaded_chunks=%d expected_chunks=%d", id, len(session.UploadedChunks), expectedChunks)
		return nil, &UploadIncompleteError{
			UploadedChunks: len(session.UploadedChunks),
			ExpectedChunks: expectedChunks,
		}
	}
	for i := 0; i < expectedChunks; i++ {
		if !containsInt(session.UploadedChunks, i) {
			log.Printf("level=warn component=upload event=complete_missing_chunk upload_id=%s missing_chunk=%d expected_chunks=%d", id, i, expectedChunks)
			return nil, &UploadIncompleteError{
				UploadedChunks: len(session.UploadedChunks),
				ExpectedChunks: expectedChunks,
			}
		}
	}
	replacedBytes := int64(0)
	if resolution.ExistingFile != nil {
		replacedBytes = resolution.ExistingFile.Size
	}
	if err := s.capacity.Check(ctx, CapacityRequest{
		LogicalBytes:         session.FileSize,
		ReplacedLogicalBytes: replacedBytes,
		PhysicalNeedBytes:    session.FileSize,
		TempNeedBytes:        session.FileSize,
	}); err != nil {
		return nil, err
	}

	if err := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusMerging); err != nil {
		log.Printf("level=error component=upload event=complete_status_failed upload_id=%s target_status=merging err=%q", id, err)
		return nil, err
	}
	failMerge := func(err error) (*FileMutationResult, error) {
		if statusErr := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusFailed); statusErr != nil {
			log.Printf("level=error component=upload event=complete_status_failed upload_id=%s target_status=failed err=%q", id, statusErr)
		}
		return nil, err
	}
	writeChunks := func(out io.Writer) error {
		for i := 0; i < expectedChunks; i++ {
			in, err := os.Open(s.chunkPath(id, i))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(out, in)
			closeErr := in.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
		return nil
	}

	var result *FileMutationResult
	for conflictAttempt := 0; ; conflictAttempt++ {
		targetName = resolution.Resolved
		log.Printf("level=info component=upload event=merge_begin upload_id=%s file_name=%q file_size=%d expected_chunks=%d dest_path=%q conflict_attempt=%d",
			id, targetName, session.FileSize, expectedChunks, session.DestPath, conflictAttempt)
		fileID := uuid.NewString()
		storageRel := s.fileStore.BuildStorageRel(fileID, session.DestPath, targetName)
		mutationKind := model.FileMutationKindUploadCreate
		var targetFile *model.File
		if policy == ConflictReplace && resolution.ExistingFile != nil {
			fileID = resolution.ExistingFile.ID
			storageRel = resolution.ExistingFile.StoragePath
			targetFile = resolution.ExistingFile
			mutationKind = model.FileMutationKindUploadReplace
		}
		file := &model.File{
			ID:          fileID,
			Name:        targetName,
			Path:        CleanVirtualPath(session.DestPath),
			StoragePath: filepath.ToSlash(storageRel),
			Size:        session.FileSize,
			Status:      model.FileStatusUploaded,
			ChunkCount:  expectedChunks,
		}
		result, err = s.mutations.Apply(ctx, FileMutationInput{
			Kind:       mutationKind,
			File:       file,
			TargetFile: targetFile,
			UploadID:   session.ID,
		}, writeChunks)
		if err == nil {
			break
		}
		log.Printf("level=warn component=upload event=complete_mutation_failed upload_id=%s file_id=%s storage_path=%q conflict_attempt=%d err=%q",
			id, fileID, storageRel, conflictAttempt, err)
		var conflict *FileConflictError
		if policy != ConflictRename || !errors.As(err, &conflict) {
			return failMerge(err)
		}
		if conflictAttempt+1 >= maxConflictRenameAttempts {
			return failMerge(&NameExhaustedError{
				Path:        session.DestPath,
				Name:        session.FileName,
				MaxAttempts: maxConflictRenameAttempts,
			})
		}
		resolution, err = (fileConflictResolver{store: s.store}).Resolve(
			ctx,
			session.DestPath,
			session.FileName,
			ConflictRename,
		)
		if err != nil {
			return failMerge(err)
		}
		if err := s.store.UpdateUploadResolution(ctx, session.ID, resolution.Resolved, ""); err != nil {
			return failMerge(err)
		}
	}
	_ = os.RemoveAll(s.sessionDir(id))
	log.Printf("level=info component=upload event=complete upload_id=%s file_id=%s file_name=%q storage_path=%q mime_type=%q size=%d expected_chunks=%d duration_ms=%d",
		id, result.File.ID, result.File.Name, result.File.StoragePath, result.File.MimeType, result.File.Size, expectedChunks, time.Since(started).Milliseconds())
	return result, nil
}

func (s *UploadService) CleanupExpired(ctx context.Context) error {
	ids, err := s.store.DeleteExpiredUploadSessions(ctx, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, id := range ids {
		_ = os.RemoveAll(s.sessionDir(id))
	}
	if len(ids) > 0 {
		log.Printf("level=info component=upload event=cleanup_expired_complete count=%d", len(ids))
	}
	return nil
}

func (s *UploadService) sessionDir(id string) string {
	return filepath.Join(s.cfg.Storage.TempDir, id)
}

func (s *UploadService) chunkPath(id string, index int) string {
	return filepath.Join(s.sessionDir(id), fmt.Sprintf("%08d.part", index))
}

func containsInt(values []int, needle int) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func detectMime(absPath, name string) string {
	if byExt := stableMimeByExtension(name); byExt != "" {
		return byExt
	}
	if byExt := mime.TypeByExtension(path.Ext(name)); byExt != "" {
		return byExt
	}
	file, err := os.Open(absPath)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(file, buf)
	if n == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(buf[:n])
}

func stableMimeByExtension(name string) string {
	switch strings.ToLower(path.Ext(name)) {
	case ".mov":
		return "video/quicktime"
	default:
		return ""
	}
}

// ChunkIndexFromOffset parses the Upload-Offset header and returns the zero-based chunk index.
func ChunkIndexFromOffset(offsetHeader string, chunkSize int64) (int, error) {
	offset, err := strconv.ParseInt(offsetHeader, 10, 64)
	if err != nil {
		return 0, err
	}
	if offset%chunkSize != 0 {
		return 0, errors.New("upload offset must align to chunk size")
	}
	return int(offset / chunkSize), nil
}
