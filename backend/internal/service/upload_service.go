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
}

// UploadCompletion holds the result of a completed upload: the created file and its pipeline task.
type UploadCompletion struct {
	File *model.File
	Task *model.Task
}

// NewUploadService creates a new UploadService.
func NewUploadService(cfg *config.Config, store *store.Store, fileStore *FileService, pipeline *PipelineService) *UploadService {
	return &UploadService{cfg: cfg, store: store, fileStore: fileStore, pipeline: pipeline}
}

// InitUploadInput is the payload for initializing a new upload session.
type InitUploadInput struct {
	FileName string `json:"file_name"`
	FileSize int64  `json:"file_size"`
	DestPath string `json:"dest_path"`
}

func (s *UploadService) Init(ctx context.Context, input InitUploadInput) (*model.UploadSession, error) {
	started := time.Now()
	if input.FileSize <= 0 {
		return nil, errors.New("file_size must be greater than zero")
	}
	if input.FileSize > s.cfg.Storage.MaxFileSize {
		return nil, fmt.Errorf("file exceeds max size of %d bytes", s.cfg.Storage.MaxFileSize)
	}
	name := SafeName(input.FileName)
	if name == "" {
		return nil, errors.New("file_name is required")
	}

	session := &model.UploadSession{
		ID:             uuid.NewString(),
		FileName:       name,
		FileSize:       input.FileSize,
		ChunkSize:      s.cfg.Storage.ChunkSize,
		UploadedChunks: []int{},
		DestPath:       CleanVirtualPath(input.DestPath),
		Status:         model.UploadStatusUploading,
		ExpiresAt:      time.Now().UTC().Add(s.cfg.Storage.UploadTTL),
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
		return fmt.Errorf("upload session is %s", session.Status)
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
		return fmt.Errorf("upload session is %s", session.Status)
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
		return nil, fmt.Errorf("upload session is %s", session.Status)
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
	file, err := s.completeFile(ctx, id)
	if err != nil {
		return nil, err
	}
	task, err := s.pipeline.Enqueue(ctx, file)
	if err != nil {
		return nil, err
	}
	return &UploadCompletion{File: file, Task: task}, nil
}

func (s *UploadService) completeFile(ctx context.Context, id string) (*model.File, error) {
	started := time.Now()
	session, err := s.GetSession(ctx, id)
	if err != nil {
		log.Printf("level=error component=upload event=complete_lookup_failed upload_id=%s err=%q", id, err)
		return nil, err
	}
	if !canCompleteUploadStatus(session.Status) {
		log.Printf("level=warn component=upload event=complete_rejected upload_id=%s status=%q", id, session.Status)
		return nil, fmt.Errorf("upload session is %s", session.Status)
	}

	expectedChunks := int((session.FileSize + session.ChunkSize - 1) / session.ChunkSize)
	if len(session.UploadedChunks) != expectedChunks {
		log.Printf("level=warn component=upload event=complete_missing_chunks upload_id=%s uploaded_chunks=%d expected_chunks=%d", id, len(session.UploadedChunks), expectedChunks)
		return nil, fmt.Errorf("missing chunks: uploaded %d of %d", len(session.UploadedChunks), expectedChunks)
	}
	for i := 0; i < expectedChunks; i++ {
		if !containsInt(session.UploadedChunks, i) {
			log.Printf("level=warn component=upload event=complete_missing_chunk upload_id=%s missing_chunk=%d expected_chunks=%d", id, i, expectedChunks)
			return nil, fmt.Errorf("missing chunk %d", i)
		}
	}

	if err := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusMerging); err != nil {
		log.Printf("level=error component=upload event=complete_status_failed upload_id=%s target_status=merging err=%q", id, err)
		return nil, err
	}
	failMerge := func(err error) (*model.File, error) {
		if statusErr := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusFailed); statusErr != nil {
			log.Printf("level=error component=upload event=complete_status_failed upload_id=%s target_status=failed err=%q", id, statusErr)
		}
		return nil, err
	}
	log.Printf("level=info component=upload event=merge_begin upload_id=%s file_name=%q file_size=%d expected_chunks=%d dest_path=%q", id, session.FileName, session.FileSize, expectedChunks, session.DestPath)
	fileID := uuid.NewString()
	storageRel := s.fileStore.BuildStorageRel(fileID, session.DestPath, session.FileName)
	absPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(storageRel))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		log.Printf("level=error component=upload event=merge_mkdir_failed upload_id=%s file_id=%s storage_path=%q err=%q", id, fileID, storageRel, err)
		return failMerge(err)
	}
	out, err := os.Create(absPath)
	if err != nil {
		log.Printf("level=error component=upload event=merge_create_failed upload_id=%s file_id=%s storage_path=%q err=%q", id, fileID, storageRel, err)
		return failMerge(err)
	}
	defer out.Close()

	for i := 0; i < expectedChunks; i++ {
		in, err := os.Open(s.chunkPath(id, i))
		if err != nil {
			log.Printf("level=error component=upload event=merge_open_chunk_failed upload_id=%s chunk_index=%d err=%q", id, i, err)
			return failMerge(err)
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = in.Close()
			log.Printf("level=error component=upload event=merge_copy_chunk_failed upload_id=%s chunk_index=%d err=%q", id, i, err)
			return failMerge(err)
		}
		_ = in.Close()
	}
	if err := out.Sync(); err != nil {
		log.Printf("level=error component=upload event=merge_sync_failed upload_id=%s file_id=%s storage_path=%q err=%q", id, fileID, storageRel, err)
		return failMerge(err)
	}
	mimeType := detectMime(absPath, session.FileName)

	file := &model.File{
		ID:          fileID,
		Name:        session.FileName,
		Path:        CleanVirtualPath(session.DestPath),
		StoragePath: filepath.ToSlash(storageRel),
		Size:        session.FileSize,
		MimeType:    mimeType,
		Status:      model.FileStatusUploaded,
		ChunkCount:  expectedChunks,
	}
	if err := s.store.CreateFile(ctx, file); err != nil {
		log.Printf("level=error component=upload event=complete_create_file_failed upload_id=%s file_id=%s storage_path=%q err=%q", id, fileID, storageRel, err)
		return failMerge(err)
	}
	if err := s.store.UpdateUploadStatus(ctx, id, model.UploadStatusDone); err != nil {
		log.Printf("level=error component=upload event=complete_status_failed upload_id=%s target_status=done err=%q", id, err)
		return failMerge(err)
	}
	_ = os.RemoveAll(s.sessionDir(id))
	log.Printf("level=info component=upload event=complete upload_id=%s file_id=%s file_name=%q storage_path=%q mime_type=%q size=%d expected_chunks=%d duration_ms=%d",
		id, file.ID, file.Name, file.StoragePath, file.MimeType, file.Size, expectedChunks, time.Since(started).Milliseconds())
	return file, nil
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
