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
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
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
	file, err := s.createFileLocked(ctx, input)
	if err != nil {
		return nil, err
	}
	return &WebDAVPutFileResult{File: file, Created: true}, nil
}

func (s *WebDAVService) CreateFile(ctx context.Context, input WebDAVCreateFileInput) (*model.File, error) {
	if err := validateWebDAVVirtualPath(input.VirtualPath); err != nil {
		return nil, err
	}
	input.VirtualPath = CleanVirtualPath(input.VirtualPath)
	unlock := s.lockPaths(input.VirtualPath)
	defer unlock()
	return s.createFileLocked(ctx, input)
}

func (s *WebDAVService) createFileLocked(ctx context.Context, input WebDAVCreateFileInput) (*model.File, error) {
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
	defer func() { _ = os.Remove(tempPath) }()
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
	mimeType := strings.TrimSpace(input.ContentType)
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
	result, err := s.mutations.applyLocked(ctx, FileMutationInput{
		Kind: model.FileMutationKindUploadCreate,
		File: file,
	}, func(writer io.Writer) error {
		source, err := os.Open(tempPath)
		if err != nil {
			return err
		}
		defer source.Close()
		_, err = io.Copy(writer, source)
		return err
	})
	if err != nil {
		var conflict *FileConflictError
		if errors.As(err, &conflict) {
			return nil, ErrPathConflict
		}
		return nil, err
	}
	file = result.File
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
	defer func() { _ = os.Remove(tempPath) }()
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

	mimeType := strings.TrimSpace(input.ContentType)
	result, err := s.mutations.applyLocked(ctx, FileMutationInput{
		Kind: model.FileMutationKindUploadReplace,
		File: &model.File{
			ID:          existing.ID,
			Name:        existing.Name,
			Path:        existing.Path,
			StoragePath: existing.StoragePath,
			Size:        stat.Size(),
			MimeType:    mimeType,
			Status:      model.FileStatusUploaded,
			ChunkCount:  0,
		},
		TargetFile:    existing,
		VersionSource: model.FileVersionSourceWebDAVPut,
	}, func(writer io.Writer) error {
		source, err := os.Open(tempPath)
		if err != nil {
			return err
		}
		defer source.Close()
		_, err = io.Copy(writer, source)
		return err
	})
	if err != nil {
		var conflict *FileConflictError
		if errors.As(err, &conflict) {
			return nil, ErrPathConflict
		}
		return nil, err
	}
	file := result.File
	log.Printf("level=info component=webdav event=put_overwrite_complete file_id=%s path=%q name=%q storage_path=%q size=%d duration_ms=%d",
		file.ID, file.Path, file.Name, file.StoragePath, file.Size, time.Since(started).Milliseconds())
	return file, nil
}
func (s *WebDAVService) checkUploadQuota(ctx context.Context, uploadSize int64, replacedBytes ...int64) error {
	replaced := int64(0)
	for _, bytes := range replacedBytes {
		replaced += bytes
	}
	return s.capacity.Check(ctx, CapacityRequest{
		LogicalBytes:         uploadSize,
		ReplacedLogicalBytes: replaced,
		PhysicalNeedBytes:    uploadSize,
		TempNeedBytes:        uploadSize,
	})
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

func moveWebDAVUploadIntoPlace(tempPath, absPath string) error {
	return moveWebDAVUploadIntoPlaceWithRename(tempPath, absPath, os.Rename)
}

func moveWebDAVUploadIntoPlaceWithRename(tempPath, absPath string, rename func(string, string) error) error {
	if err := rename(tempPath, absPath); err == nil {
		return nil
	} else if !errors.Is(err, syscall.EXDEV) {
		return err
	}

	temp, err := os.CreateTemp(filepath.Dir(absPath), "."+filepath.Base(absPath)+".*.tmp")
	if err != nil {
		return err
	}
	fallbackPath := temp.Name()
	cleanupFallback := true
	defer func() {
		_ = temp.Close()
		if cleanupFallback {
			_ = os.Remove(fallbackPath)
		}
	}()

	source, err := os.Open(tempPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(temp, source); err != nil {
		_ = source.Close()
		return err
	}
	if err := source.Close(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(fallbackPath, absPath); err != nil {
		return err
	}
	cleanupFallback = false
	return os.Remove(tempPath)
}
