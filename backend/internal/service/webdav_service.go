package service

import (
	"context"
	"errors"
	"path"
	"path/filepath"
	"strings"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

var (
	ErrFileTooLarge        = errors.New("file too large")
	ErrInsufficientStorage = errors.New("insufficient storage")
	ErrPreconditionFailed  = errors.New("precondition failed")
	ErrUnsupportedResource = errors.New("unsupported resource")
	ErrInvalidWebDAVPath   = errors.New("invalid webdav path")
)

// WebDAVResource is a File or the virtual root folder exposed through WebDAV.
type WebDAVResource struct {
	VirtualPath string
	File        *model.File
	Root        bool
}

func (r *WebDAVResource) IsDir() bool {
	return r != nil && (r.Root || (r.File != nil && r.File.IsDir))
}

// WebDAVService resolves MemoDrive virtual paths for the WebDAV endpoint.
type WebDAVService struct {
	cfg      *config.Config
	store    *store.Store
	pipeline *PipelineService
	locks    *webDAVPathLocks
}

func NewWebDAVService(cfg *config.Config, store *store.Store, pipelines ...*PipelineService) *WebDAVService {
	var pipeline *PipelineService
	if len(pipelines) > 0 {
		pipeline = pipelines[0]
	}
	return &WebDAVService{cfg: cfg, store: store, pipeline: pipeline, locks: newWebDAVPathLocks()}
}

func (s *WebDAVService) Resolve(ctx context.Context, virtualPath string) (*WebDAVResource, error) {
	if err := validateWebDAVVirtualPath(virtualPath); err != nil {
		return nil, err
	}
	virtualPath = CleanVirtualPath(virtualPath)
	if virtualPath == "/" {
		return &WebDAVResource{VirtualPath: "/", Root: true}, nil
	}
	currentPath := "/"
	segments := strings.Split(strings.TrimPrefix(virtualPath, "/"), "/")
	for i, segment := range segments {
		file, err := s.store.GetActiveByPath(ctx, currentPath, segment)
		if errors.Is(err, store.ErrPathConflict) {
			return nil, ErrPathConflict
		}
		if err != nil {
			return nil, err
		}
		canonicalPath := path.Join(file.Path, file.Name)
		if i == len(segments)-1 {
			return &WebDAVResource{VirtualPath: canonicalPath, File: file}, nil
		}
		if !file.IsDir {
			return nil, store.ErrNotFound
		}
		currentPath = canonicalPath
	}
	return nil, store.ErrNotFound
}

func (s *WebDAVService) ListChildren(ctx context.Context, virtualPath string) ([]model.File, error) {
	resource, err := s.Resolve(ctx, virtualPath)
	if err != nil {
		return nil, err
	}
	if !resource.IsDir() {
		return nil, store.ErrNotFound
	}
	return s.store.ListFiles(ctx, resource.VirtualPath, "")
}

func (s *WebDAVService) DownloadPath(resource *WebDAVResource) (*model.File, string, error) {
	if resource == nil || resource.File == nil || resource.IsDir() {
		return nil, "", store.ErrNotFound
	}
	return resource.File, filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(resource.File.StoragePath)), nil
}

func (s *WebDAVService) StorageUsage(ctx context.Context) (*StorageUsage, error) {
	used, err := s.store.TotalActiveFileSize(ctx)
	if err != nil {
		return nil, err
	}
	total, err := filesystemTotalBytes(s.cfg.Storage.Root)
	if err != nil {
		return nil, err
	}
	return &StorageUsage{
		UsedBytes:  used,
		TotalBytes: total,
	}, nil
}
