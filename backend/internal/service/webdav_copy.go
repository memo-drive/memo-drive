package service

import (
	"context"
	"errors"
	"path"

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
	if err := validateWebDAVVirtualPath(input.DestinationPath); err != nil {
		return nil, err
	}
	destinationPath := CleanVirtualPath(input.DestinationPath)
	if destinationPath == "/" {
		return nil, ErrPathConflict
	}
	unlock := s.lockPaths(input.Source.VirtualPath, destinationPath)
	defer unlock()
	// Resolve again after acquiring the path locks. Another WebDAV operation may
	// have moved or replaced the resource between the handler's initial resolve
	// and this critical section, making the cached File ID stale.
	source, err := s.Resolve(ctx, input.Source.VirtualPath)
	if err != nil {
		if errors.Is(err, ErrPathConflict) {
			return nil, err
		}
		return nil, store.ErrNotFound
	}
	if source.File == nil {
		return nil, store.ErrNotFound
	}
	destinationExists := false
	if existing, err := s.Resolve(ctx, destinationPath); err == nil && existing != nil {
		destinationExists = true
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

	policy := ConflictReject
	if destinationExists && input.Overwrite {
		policy = ConflictReplace
	}
	result, err := s.files.copy(ctx, source.File.ID, FileCopyInput{
		Path:           CleanVirtualPath(path.Dir(destinationPath)),
		Name:           path.Base(destinationPath),
		ConflictPolicy: policy,
	}, false)
	if err != nil {
		var conflict *FileConflictError
		if !input.Overwrite && errors.As(err, &conflict) {
			return nil, ErrPreconditionFailed
		}
		return nil, err
	}
	return &WebDAVCopyResult{File: result.File, Created: result.Created}, nil
}
