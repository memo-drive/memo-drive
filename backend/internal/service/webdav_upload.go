package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

type WebDAVCreateFileInput struct {
	VirtualPath   string
	Body          io.Reader
	ContentLength int64
	ContentType   string
}

type WebDAVPutFileResult struct {
	File    *model.File
	Created bool
}

func (s *WebDAVService) PutFile(ctx context.Context, input WebDAVCreateFileInput) (*WebDAVPutFileResult, error) {
	if err := validateWebDAVVirtualPath(input.VirtualPath); err != nil {
		return nil, err
	}
	virtualPath := CleanVirtualPath(input.VirtualPath)
	input.VirtualPath = virtualPath
	unlock := s.lockPaths(virtualPath)
	defer unlock()
	return s.putFile(ctx, input)
}

func (s *WebDAVService) putFile(ctx context.Context, input WebDAVCreateFileInput) (*WebDAVPutFileResult, error) {
	if err := validateWebDAVVirtualPath(input.VirtualPath); err != nil {
		return nil, err
	}
	virtualPath := CleanVirtualPath(input.VirtualPath)
	input.VirtualPath = virtualPath
	if existing, err := s.Resolve(ctx, virtualPath); err == nil && existing != nil {
		if existing.IsDir() {
			return nil, ErrPathConflict
		}
		file, err := s.overwriteFile(ctx, input, existing.File)
		if err != nil {
			return nil, err
		}
		return &WebDAVPutFileResult{File: file}, nil
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	file, err := s.CreateFile(ctx, input)
	if err != nil {
		return nil, err
	}
	return &WebDAVPutFileResult{File: file, Created: true}, nil
}

func (s *WebDAVService) CreateFile(ctx context.Context, input WebDAVCreateFileInput) (*model.File, error) {
	started := time.Now()
	if err := validateWebDAVVirtualPath(input.VirtualPath); err != nil {
		return nil, err
	}
	virtualPath := CleanVirtualPath(input.VirtualPath)
	if virtualPath == "/" {
		return nil, ErrPathConflict
	}
	if input.ContentLength > s.cfg.Storage.MaxFileSize {
		return nil, ErrFileTooLarge
	}
	if input.ContentLength > 0 {
		if err := s.checkUploadQuota(ctx, input.ContentLength); err != nil {
			return nil, err
		}
	}
	parentPath := CleanVirtualPath(path.Dir(virtualPath))
	name := path.Base(virtualPath)
	parent, err := s.Resolve(ctx, parentPath)
	if err != nil {
		if errors.Is(err, ErrPathConflict) {
			return nil, err
		}
		return nil, store.ErrNotFound
	}
	if !parent.IsDir() {
		return nil, store.ErrNotFound
	}
	if existing, err := s.Resolve(ctx, virtualPath); err == nil && existing != nil {
		return nil, ErrPathConflict
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	tempPath, err := s.writeUploadTemp(input.Body)
	if err != nil {
		return nil, err
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()
	stat, err := os.Stat(tempPath)
	if err != nil {
		return nil, err
	}
	if stat.Size() > s.cfg.Storage.MaxFileSize {
		return nil, ErrFileTooLarge
	}
	if err := s.checkUploadQuota(ctx, stat.Size()); err != nil {
		return nil, err
	}

	fileID := uuid.NewString()
	storageRel := s.buildStorageRel(fileID, parent.VirtualPath, name)
	absPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(storageRel))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, absPath); err != nil {
		return nil, err
	}
	cleanupTemp = false
	cleanupStored := true
	defer func() {
		if cleanupStored {
			_ = os.Remove(absPath)
		}
	}()

	mimeType := strings.TrimSpace(input.ContentType)
	if mimeType == "" {
		mimeType = detectMime(absPath, name)
	}
	file := &model.File{
		ID:          fileID,
		Name:        name,
		Path:        parent.VirtualPath,
		StoragePath: filepath.ToSlash(storageRel),
		Size:        stat.Size(),
		MimeType:    mimeType,
		Status:      model.FileStatusUploaded,
		ChunkCount:  1,
	}
	if err := s.store.CreateFile(ctx, file); err != nil {
		if errors.Is(err, store.ErrPathConflict) {
			return nil, ErrPathConflict
		}
		return nil, err
	}
	cleanupStored = false
	if s.pipeline != nil {
		if _, err := s.pipeline.Enqueue(ctx, file); err != nil {
			log.Printf("level=warn component=webdav event=pipeline_enqueue_failed file_id=%s name=%q err=%q", file.ID, file.Name, err)
		}
	}
	log.Printf("level=info component=webdav event=put_create_complete file_id=%s path=%q name=%q storage_path=%q size=%d duration_ms=%d",
		file.ID, file.Path, file.Name, file.StoragePath, file.Size, time.Since(started).Milliseconds())
	return file, nil
}

func (s *WebDAVService) overwriteFile(ctx context.Context, input WebDAVCreateFileInput, existing *model.File) (*model.File, error) {
	started := time.Now()
	if input.ContentLength > s.cfg.Storage.MaxFileSize {
		return nil, ErrFileTooLarge
	}
	if input.ContentLength > 0 {
		if err := s.checkUploadQuota(ctx, input.ContentLength, existing.Size); err != nil {
			return nil, err
		}
	}
	tempPath, err := s.writeUploadTemp(input.Body)
	if err != nil {
		return nil, err
	}
	cleanupTemp := true
	defer func() {
		if cleanupTemp {
			_ = os.Remove(tempPath)
		}
	}()
	stat, err := os.Stat(tempPath)
	if err != nil {
		return nil, err
	}
	if stat.Size() > s.cfg.Storage.MaxFileSize {
		return nil, ErrFileTooLarge
	}
	if err := s.checkUploadQuota(ctx, stat.Size(), existing.Size); err != nil {
		return nil, err
	}

	absPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(existing.StoragePath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.Rename(tempPath, absPath); err != nil {
		return nil, err
	}
	cleanupTemp = false

	mimeType := strings.TrimSpace(input.ContentType)
	if mimeType == "" {
		mimeType = detectMime(absPath, existing.Name)
	}
	if err := s.store.UpdateFileContent(ctx, existing.ID, stat.Size(), mimeType, model.FileStatusUploaded, 0); err != nil {
		return nil, err
	}
	if err := s.cleanupPreviousIndex(ctx, existing); err != nil {
		return nil, err
	}
	file, err := s.store.GetFile(ctx, existing.ID)
	if err != nil {
		return nil, err
	}
	if s.pipeline != nil {
		if _, err := s.pipeline.Enqueue(ctx, file); err != nil {
			log.Printf("level=warn component=webdav event=pipeline_enqueue_failed file_id=%s name=%q err=%q", file.ID, file.Name, err)
		}
	}
	log.Printf("level=info component=webdav event=put_overwrite_complete file_id=%s path=%q name=%q storage_path=%q size=%d duration_ms=%d",
		file.ID, file.Path, file.Name, file.StoragePath, file.Size, time.Since(started).Milliseconds())
	return file, nil
}

func (s *WebDAVService) cleanupPreviousIndex(ctx context.Context, file *model.File) error {
	if file == nil || file.IsDir {
		return nil
	}
	if file.ChunkCount > 0 && s.pipeline != nil && s.pipeline.vectorDB != nil {
		ids := indexing.ChunkIDs(file.ID, file.ChunkCount)
		if err := s.pipeline.vectorDB.Delete(ctx, vectordb.DefaultCollection, ids); err != nil {
			log.Printf("level=warn component=webdav event=vector_cleanup_failed file_id=%s chunk_count=%d err=%q", file.ID, file.ChunkCount, err)
		}
	}
	if err := s.store.DeleteChunksByFileID(ctx, file.ID); err != nil {
		return err
	}
	return s.store.DeleteMetadataByFileID(ctx, file.ID)
}

func (s *WebDAVService) checkUploadQuota(ctx context.Context, uploadSize int64, replacedBytes ...int64) error {
	used, err := s.store.TotalActiveFileSize(ctx)
	if err != nil {
		return err
	}
	for _, bytes := range replacedBytes {
		used -= bytes
	}
	if used < 0 {
		used = 0
	}
	total, err := filesystemTotalBytes(s.cfg.Storage.Root)
	if err != nil {
		return err
	}
	if used >= total {
		return ErrInsufficientStorage
	}
	if uploadSize > total-used {
		return ErrInsufficientStorage
	}
	return nil
}

func (s *WebDAVService) writeUploadTemp(body io.Reader) (string, error) {
	tempDir := filepath.Join(s.cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(tempDir, "*.upload")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		_ = temp.Close()
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	limited := &io.LimitedReader{R: body, N: s.cfg.Storage.MaxFileSize + 1}
	written, err := io.Copy(temp, limited)
	if err != nil {
		return "", err
	}
	if written > s.cfg.Storage.MaxFileSize {
		return "", ErrFileTooLarge
	}
	if err := temp.Sync(); err != nil {
		return "", err
	}
	cleanup = false
	return tempPath, nil
}

func (s *WebDAVService) buildStorageRel(fileID, destPath, fileName string) string {
	ext := path.Ext(fileName)
	base := strings.TrimSuffix(SafeName(fileName), ext)
	name := fmt.Sprintf("%s-%s%s", base, fileID, ext)
	return filepath.ToSlash(path.Join(strings.TrimPrefix(CleanVirtualPath(destPath), "/"), name))
}
