package service

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func (s *WebDAVService) CreateFolder(ctx context.Context, virtualPath string) (*model.File, error) {
	if err := validateWebDAVVirtualPath(virtualPath); err != nil {
		return nil, err
	}
	virtualPath = CleanVirtualPath(virtualPath)
	unlock := s.lockPaths(virtualPath)
	defer unlock()
	if virtualPath == "/" {
		return nil, ErrPathConflict
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
	} else if errors.Is(err, ErrPathConflict) {
		return nil, err
	} else if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}

	storageRel := filepath.ToSlash(path.Join(strings.TrimPrefix(parent.VirtualPath, "/"), name))
	absPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(storageRel))
	physicalExisted := false
	if _, err := os.Stat(absPath); err == nil {
		physicalExisted = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := os.MkdirAll(absPath, 0o755); err != nil {
		return nil, err
	}
	cleanupStored := true
	defer func() {
		if cleanupStored && !physicalExisted {
			_ = os.Remove(absPath)
		}
	}()

	file := &model.File{
		ID:          uuid.NewString(),
		Name:        name,
		Path:        parent.VirtualPath,
		StoragePath: storageRel,
		IsDir:       true,
		Status:      model.FileStatusReady,
	}
	if err := s.store.CreateFile(ctx, file); err != nil {
		if errors.Is(err, store.ErrPathConflict) {
			return nil, ErrPathConflict
		}
		return nil, err
	}
	cleanupStored = false
	return file, nil
}
