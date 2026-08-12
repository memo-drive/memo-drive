package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
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
	unlock := sharedFilePathLocks.lock(virtualTarget(file.Path, file.Name))
	defer unlock()
	file, err = s.store.GetFile(ctx, id)
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
	unlock := sharedFilePathLocks.lock(virtualTarget(file.Path, file.Name))
	defer unlock()
	file, err = s.store.GetFile(ctx, id)
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

	mutations := s.markdownMutations()
	result, err := mutations.applyLocked(ctx, FileMutationInput{
		Kind: model.FileMutationKindUploadReplace,
		File: &model.File{
			ID:          file.ID,
			Name:        file.Name,
			Path:        file.Path,
			StoragePath: file.StoragePath,
			Size:        int64(len([]byte(content))),
			MimeType:    "text/markdown",
			Status:      model.FileStatusUploaded,
			ChunkCount:  0,
		},
		TargetFile:    file,
		VersionSource: model.FileVersionSourceMarkdownSave,
	}, func(writer io.Writer) error {
		_, err := io.WriteString(writer, content)
		return err
	})
	if err != nil {
		return nil, err
	}
	updated := result.File
	nextVersion, err := markdownContentVersion(absPath)
	if err != nil {
		return nil, err
	}
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
	unlock := sharedFilePathLocks.lock(virtualTarget(dirPath, name))
	defer unlock()
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
	result, err := s.markdownMutations().applyLocked(ctx, FileMutationInput{
		Kind: model.FileMutationKindUploadCreate,
		File: file,
	}, func(io.Writer) error {
		return nil
	})
	if err != nil {
		return nil, mapStorePathConflict(err)
	}
	return result.File, nil
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

func (s *FileService) markdownMutations() *FileMutationService {
	mutations := NewFileMutationService(s.cfg, s.store, s.pipeline)
	mutations.vectorDB = s.vectorDB
	return mutations
}
