package service

import (
	"archive/zip"
	"context"
	"io"
	"os"
	"path"
	"strings"

	"github.com/memodrive/backend/internal/model"
)

// FolderZIP is a preflighted virtual Folder tree that can be written directly to a stream.
type FolderZIP struct {
	service *FileService
	root    *model.File
	entries []model.File
	names   []string
}

type FolderArchiveLimitError struct {
	Nodes                int
	MaxNodes             int
	UncompressedBytes    int64
	MaxUncompressedBytes int64
}

func (e *FolderArchiveLimitError) Error() string {
	return "folder archive exceeds configured limits"
}

type UnsafeArchivePathError struct {
	FileID string
}

type FolderArchiveReadError struct {
	FileID string
	Err    error
}

func (e *FolderArchiveReadError) Error() string {
	return "folder archive source cannot be read"
}

func (e *FolderArchiveReadError) Unwrap() error {
	return e.Err
}

func (e *UnsafeArchivePathError) Error() string {
	return "folder archive contains an unsafe entry path"
}

func (s *FileService) PrepareFolderZIP(ctx context.Context, id string) (*FolderZIP, error) {
	root, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	if !root.IsDir {
		return nil, ErrUnsupportedResource
	}
	rootVirtual := CleanVirtualPath(path.Join(root.Path, root.Name))
	entries, err := s.store.ListDescendants(ctx, rootVirtual)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(entries))
	for i := range entries {
		names[i], err = folderZIPEntryName(rootVirtual, &entries[i])
		if err != nil {
			return nil, err
		}
		if entries[i].IsDir {
			continue
		}
		info, statErr := os.Stat(s.absStoragePath(entries[i].StoragePath))
		if statErr != nil || !info.Mode().IsRegular() || info.Size() != entries[i].Size {
			if statErr == nil {
				statErr = os.ErrInvalid
			}
			return nil, &FolderArchiveReadError{FileID: entries[i].ID, Err: statErr}
		}
	}
	maxNodes := s.cfg.Storage.FolderZIPMaxNodes
	if maxNodes <= 0 {
		maxNodes = 10000
	}
	nodes := len(entries) + 1
	if nodes > maxNodes {
		return nil, &FolderArchiveLimitError{Nodes: nodes, MaxNodes: maxNodes}
	}
	maxBytes := s.cfg.Storage.FolderZIPMaxUncompressedBytes
	if maxBytes <= 0 {
		maxBytes = 20 * 1024 * 1024 * 1024
	}
	var totalBytes int64
	for i := range entries {
		if entries[i].IsDir {
			continue
		}
		if entries[i].Size < 0 || entries[i].Size > maxBytes-totalBytes {
			return nil, &FolderArchiveLimitError{
				Nodes:                nodes,
				MaxNodes:             maxNodes,
				UncompressedBytes:    totalBytes + entries[i].Size,
				MaxUncompressedBytes: maxBytes,
			}
		}
		totalBytes += entries[i].Size
	}
	return &FolderZIP{service: s, root: root, entries: entries, names: names}, nil
}

func (a *FolderZIP) Name() string {
	return a.root.Name + ".zip"
}

func (a *FolderZIP) Write(ctx context.Context, output io.Writer) error {
	archive := zip.NewWriter(output)
	for i := range a.entries {
		if err := ctx.Err(); err != nil {
			_ = archive.Close()
			return err
		}
		entry := &a.entries[i]
		entryName := a.names[i]
		if entry.IsDir {
			if _, err := archive.Create(entryName + "/"); err != nil {
				_ = archive.Close()
				return err
			}
			continue
		}
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: entryName, Method: zip.Deflate})
		if err != nil {
			_ = archive.Close()
			return err
		}
		file, err := os.Open(a.service.absStoragePath(entry.StoragePath))
		if err != nil {
			_ = archive.Close()
			return err
		}
		_, copyErr := io.Copy(writer, &contextReader{ctx: ctx, reader: file})
		closeErr := file.Close()
		if copyErr != nil {
			_ = archive.Close()
			return copyErr
		}
		if closeErr != nil {
			_ = archive.Close()
			return closeErr
		}
	}
	return archive.Close()
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func folderZIPEntryName(rootVirtual string, entry *model.File) (string, error) {
	if entry == nil || entry.ID == "" || entry.Name == "" {
		return "", &UnsafeArchivePathError{}
	}
	if CleanVirtualPath(entry.Path) != entry.Path ||
		(entry.Path != rootVirtual && !strings.HasPrefix(entry.Path, rootVirtual+"/")) {
		return "", &UnsafeArchivePathError{FileID: entry.ID}
	}
	if path.Base(entry.Name) != entry.Name || SafeName(entry.Name) != entry.Name || strings.ContainsRune(entry.Name, '\x00') {
		return "", &UnsafeArchivePathError{FileID: entry.ID}
	}
	relativeParent := strings.TrimPrefix(strings.TrimPrefix(entry.Path, rootVirtual), "/")
	entryName := path.Join(relativeParent, entry.Name)
	if entryName == "." || entryName == ".." || path.IsAbs(entryName) || strings.HasPrefix(entryName, "../") {
		return "", &UnsafeArchivePathError{FileID: entry.ID}
	}
	return entryName, nil
}
