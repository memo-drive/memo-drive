package service

import (
	"context"

	"github.com/memodrive/backend/internal/store"
)

func (s *WebDAVService) Delete(ctx context.Context, resource *WebDAVResource) error {
	if resource == nil || resource.File == nil {
		return store.ErrNotFound
	}
	unlock := s.lockPaths(resource.VirtualPath)
	defer unlock()
	return NewFileService(s.cfg, s.store, nil).SoftDelete(ctx, resource.File.ID)
}
