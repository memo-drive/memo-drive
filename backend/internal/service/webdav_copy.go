package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

type WebDAVCopyInput struct {
	Source          *WebDAVResource
	DestinationPath string
	Overwrite       bool
}

type WebDAVCopyResult struct {
	File    *model.File
	Created bool
}

func (s *WebDAVService) Copy(ctx context.Context, input WebDAVCopyInput) (*WebDAVCopyResult, error) {
	if input.Source == nil || input.Source.File == nil {
		return nil, store.ErrNotFound
	}
	if input.Source.IsDir() {
		return nil, ErrUnsupportedResource
	}
	if err := validateWebDAVVirtualPath(input.DestinationPath); err != nil {
		return nil, err
	}
	destinationPath := CleanVirtualPath(input.DestinationPath)
	if destinationPath == "/" {
		return nil, ErrPathConflict
	}
	unlock := s.lockPaths(input.Source.VirtualPath, destinationPath)
	defer unlock()
	if existing, err := s.Resolve(ctx, destinationPath); err == nil && existing != nil {
		if !input.Overwrite {
			return nil, ErrPreconditionFailed
		}
		if existing.IsDir() {
			return nil, ErrPathConflict
		}
	} else if errors.Is(err, ErrPathConflict) {
		return nil, err
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	source := input.Source.File
	sourcePath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(source.StoragePath))
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return nil, err
	}
	defer sourceFile.Close()

	result, err := s.putFile(ctx, WebDAVCreateFileInput{
		VirtualPath:   destinationPath,
		Body:          sourceFile,
		ContentLength: source.Size,
		ContentType:   source.MimeType,
	})
	if err != nil {
		return nil, err
	}
	return &WebDAVCopyResult{File: result.File, Created: result.Created}, nil
}
