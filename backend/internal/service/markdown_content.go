package service

import (
	"context"
	"errors"
	"fmt"
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

const MarkdownContentMaxBytes = 1 * 1024 * 1024

var ErrMarkdownConflict = errors.New("markdown content conflict")

type MarkdownContent struct {
	File      *model.File `json:"file"`
	Content   string      `json:"content"`
	UpdatedAt string      `json:"updated_at"`
}

func (s *FileService) MarkdownContent(ctx context.Context, id string) (*MarkdownContent, error) {
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := ensureEditableMarkdown(file); err != nil {
		return nil, err
	}
	if file.Size > MarkdownContentMaxBytes {
		return nil, ErrFileTooLarge
	}
	body, err := os.ReadFile(s.absStoragePath(file.StoragePath))
	if err != nil {
		return nil, err
	}
	if len(body) > MarkdownContentMaxBytes {
		return nil, ErrFileTooLarge
	}
	version, err := markdownContentVersion(s.absStoragePath(file.StoragePath))
	if err != nil {
		return nil, err
	}
	return &MarkdownContent{
		File:      file,
		Content:   strings.ToValidUTF8(string(body), ""),
		UpdatedAt: version,
	}, nil
}

func (s *FileService) UpdateMarkdownContent(ctx context.Context, id, content, baseUpdatedAt string) (*MarkdownContent, error) {
	if len([]byte(content)) > MarkdownContentMaxBytes {
		return nil, ErrFileTooLarge
	}
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := ensureEditableMarkdown(file); err != nil {
		return nil, err
	}
	absPath := s.absStoragePath(file.StoragePath)
	version, err := markdownContentVersion(absPath)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(baseUpdatedAt) == "" || baseUpdatedAt != version {
		return nil, ErrMarkdownConflict
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return nil, err
	}
	if err := s.store.UpdateFileContent(ctx, file.ID, int64(len([]byte(content))), "text/markdown", model.FileStatusUploaded, 0); err != nil {
		return nil, err
	}
	if err := s.cleanupMarkdownIndex(ctx, file); err != nil {
		return nil, err
	}
	updated, err := s.store.GetFile(ctx, file.ID)
	if err != nil {
		return nil, err
	}
	nextVersion, err := markdownContentVersion(absPath)
	if err != nil {
		return nil, err
	}
	s.enqueueMarkdownPipeline(ctx, updated)
	return &MarkdownContent{
		File:      updated,
		Content:   content,
		UpdatedAt: nextVersion,
	}, nil
}

func (s *FileService) CreateMarkdownFile(ctx context.Context, dirPath, name string) (*model.File, error) {
	name = markdownFileName(name)
	if name == "" {
		return nil, errors.New("markdown file name is required")
	}
	dirPath = CleanVirtualPath(dirPath)
	if err := s.ensureMarkdownParent(ctx, dirPath); err != nil {
		return nil, err
	}
	exists, err := s.store.ExistsAtPath(ctx, dirPath, name)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrPathConflict
	}

	fileID := uuid.NewString()
	storageRel := s.BuildStorageRel(fileID, dirPath, name)
	absPath := s.absStoragePath(storageRel)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(absPath, nil, 0o644); err != nil {
		return nil, err
	}
	cleanupStored := true
	defer func() {
		if cleanupStored {
			_ = os.Remove(absPath)
		}
	}()

	file := &model.File{
		ID:          fileID,
		Name:        name,
		Path:        dirPath,
		StoragePath: filepath.ToSlash(storageRel),
		Size:        0,
		MimeType:    "text/markdown",
		Status:      model.FileStatusUploaded,
		ChunkCount:  0,
	}
	if err := s.store.CreateFile(ctx, file); err != nil {
		return nil, mapStorePathConflict(err)
	}
	cleanupStored = false
	s.enqueueMarkdownPipeline(ctx, file)
	return file, nil
}

func (s *FileService) ensureMarkdownParent(ctx context.Context, dirPath string) error {
	if dirPath == "/" {
		return nil
	}
	parentPath := CleanVirtualPath(path.Dir(dirPath))
	name := path.Base(dirPath)
	parent, err := s.store.GetActiveByPath(ctx, parentPath, name)
	if err != nil {
		return err
	}
	if !parent.IsDir {
		return store.ErrNotFound
	}
	return nil
}

func ensureEditableMarkdown(file *model.File) error {
	if file == nil {
		return store.ErrNotFound
	}
	if file.IsDir {
		return ErrUnsupportedResource
	}
	if isMarkdownName(file.Name) || strings.EqualFold(strings.TrimSpace(file.MimeType), "text/markdown") {
		return nil
	}
	return ErrUnsupportedResource
}

func isMarkdownName(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".md" || ext == ".markdown"
}

func markdownFileName(name string) string {
	name = SafeName(name)
	if name == "" {
		return ""
	}
	if !isMarkdownName(name) {
		name += ".md"
	}
	return name
}

func markdownTimestamp(value time.Time) string {
	return value.UTC().Format(time.RFC3339Nano)
}

func markdownContentVersion(absPath string) (string, error) {
	stat, err := os.Stat(absPath)
	if err != nil {
		return "", err
	}
	return markdownTimestamp(stat.ModTime()), nil
}

func (s *FileService) cleanupMarkdownIndex(ctx context.Context, file *model.File) error {
	if file == nil || file.IsDir {
		return nil
	}
	if file.ChunkCount > 0 && s.vectorDB != nil {
		ids := indexing.ChunkIDs(file.ID, file.ChunkCount)
		if err := s.vectorDB.Delete(ctx, vectordb.DefaultCollection, ids); err != nil {
			log.Printf("level=warn component=file event=markdown_vector_cleanup_failed file_id=%s chunk_count=%d err=%q", file.ID, file.ChunkCount, err)
		}
	}
	if err := s.store.DeleteChunksByFileID(ctx, file.ID); err != nil {
		return fmt.Errorf("delete markdown chunks: %w", err)
	}
	if err := s.store.DeleteMetadataByFileID(ctx, file.ID); err != nil {
		return fmt.Errorf("delete markdown metadata: %w", err)
	}
	return nil
}

func (s *FileService) enqueueMarkdownPipeline(ctx context.Context, file *model.File) {
	if s.pipeline == nil || file == nil {
		return
	}
	if _, err := s.pipeline.Enqueue(ctx, file); err != nil {
		log.Printf("level=warn component=file event=markdown_pipeline_enqueue_failed file_id=%s name=%q err=%q", file.ID, file.Name, err)
	}
}
