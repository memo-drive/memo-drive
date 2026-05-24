package service

import (
	"context"
	"errors"
	"path"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

type WebDAVMoveInput struct {
	Source          *WebDAVResource
	DestinationPath string
	Overwrite       bool
}

type WebDAVMoveResult struct {
	File        *model.File
	Overwritten bool
}

func (s *WebDAVService) Move(ctx context.Context, input WebDAVMoveInput) (*WebDAVMoveResult, error) {
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
	parentPath := CleanVirtualPath(path.Dir(destinationPath))
	name := path.Base(destinationPath)

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
	overwritten := false
	if existing, err := s.Resolve(ctx, destinationPath); err == nil && existing != nil {
		if existing.File == nil || existing.File.ID != input.Source.File.ID {
			if !input.Overwrite {
				return nil, ErrPreconditionFailed
			}
			if existing.File == nil || existing.IsDir() || input.Source.IsDir() {
				return nil, ErrPathConflict
			}
			if err := NewFileService(s.cfg, s.store, nil).SoftDelete(ctx, existing.File.ID); err != nil {
				return nil, err
			}
			overwritten = true
		}
	} else if errors.Is(err, ErrPathConflict) {
		return nil, err
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	file, err := NewFileService(s.cfg, s.store, nil).RenameMove(ctx, input.Source.File.ID, name, parent.VirtualPath)
	if err != nil {
		return nil, err
	}
	return &WebDAVMoveResult{File: file, Overwritten: overwritten}, nil
}
