package service

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/storagefs"
	"github.com/memodrive/backend/internal/store"
)

var ErrFileVersioningDisabled = errors.New("File Versioning is disabled")

type FileVersionNotFoundError struct {
	FileID    string
	VersionID string
}

func (e *FileVersionNotFoundError) Error() string { return "File Version not found" }

func (e *FileVersionNotFoundError) Unwrap() error { return store.ErrNotFound }

func (s *FileService) ListVersions(ctx context.Context, fileID string) ([]model.FileVersion, error) {
	return s.store.ListFileVersions(ctx, fileID)
}

func (s *FileService) DeleteVersion(ctx context.Context, fileID, versionID string) error {
	version, err := s.store.GetFileVersion(ctx, fileID, versionID)
	if err != nil {
		return err
	}
	path := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(version.StoragePath))
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := storagefs.SyncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return s.store.DeleteFileVersion(ctx, fileID, versionID)
}

func (s *FileService) RestoreVersion(ctx context.Context, fileID, versionID string) (*FileMutationResult, error) {
	if !s.cfg.FileVersion.Enabled {
		return nil, ErrFileVersioningDisabled
	}
	current, err := s.store.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	version, err := s.store.GetFileVersion(ctx, fileID, versionID)
	if err != nil {
		return nil, err
	}
	unlock := sharedFilePathLocks.lock(virtualTarget(current.Path, current.Name))
	defer unlock()
	current, err = s.store.GetFile(ctx, fileID)
	if err != nil {
		return nil, err
	}
	versionPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(version.StoragePath))
	file := *current
	file.Size = version.Size
	file.MimeType = version.MimeType
	file.Status = model.FileStatusUploaded
	file.ChunkCount = 0
	mutations := NewFileMutationService(s.cfg, s.store, s.pipeline)
	return mutations.applyLocked(ctx, FileMutationInput{
		Kind:          model.FileMutationKindVersionRestore,
		File:          &file,
		TargetFile:    current,
		VersionSource: model.FileVersionSourceVersionRestore,
	}, func(writer io.Writer) error {
		source, err := os.Open(versionPath)
		if err != nil {
			return err
		}
		defer source.Close()
		_, err = io.Copy(writer, source)
		return err
	})
}

func (s *FileService) VersionDownload(ctx context.Context, fileID, versionID string) (*model.FileVersion, string, error) {
	version, err := s.store.GetFileVersion(ctx, fileID, versionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, "", &FileVersionNotFoundError{FileID: fileID, VersionID: versionID}
		}
		return nil, "", err
	}
	path := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(version.StoragePath))
	info, err := os.Stat(path)
	if err != nil {
		return nil, "", err
	}
	if !info.Mode().IsRegular() || info.Size() != version.Size {
		return nil, "", os.ErrInvalid
	}
	return version, path, nil
}
