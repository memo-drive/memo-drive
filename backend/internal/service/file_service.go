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
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

var (
	ErrPathConflict = errors.New("path conflict")
	ErrNotInTrash   = errors.New("file is not in trash")
)

type FileService struct {
	cfg      *config.Config
	store    *store.Store
	vectorDB vectordb.VectorStore
}

func NewFileService(cfg *config.Config, store *store.Store, vectorDB vectordb.VectorStore) *FileService {
	return &FileService{cfg: cfg, store: store, vectorDB: vectorDB}
}

func (s *FileService) List(ctx context.Context, dirPath, sort string) ([]model.File, error) {
	started := time.Now()
	cleanPath := CleanVirtualPath(dirPath)
	files, err := s.store.ListFiles(ctx, cleanPath, sort)
	if err != nil {
		log.Printf("level=error component=file event=list_failed path=%q sort=%q err=%q", cleanPath, sort, err)
		return nil, err
	}
	log.Printf("level=debug component=file event=list_complete path=%q sort=%q count=%d duration_ms=%d", cleanPath, sort, len(files), time.Since(started).Milliseconds())
	return files, nil
}

func (s *FileService) CreateFolder(ctx context.Context, dirPath, name string) (*model.File, error) {
	started := time.Now()
	originalName := name
	name = SafeName(name)
	if name == "" {
		return nil, errors.New("folder name is required")
	}
	dirPath = CleanVirtualPath(dirPath)
	virtual := path.Join(dirPath, name)
	rel := strings.TrimPrefix(virtual, "/")
	if rel == "." {
		rel = ""
	}
	if err := os.MkdirAll(s.absStoragePath(rel), 0o755); err != nil {
		log.Printf("level=error component=file event=create_folder_mkdir_failed path=%q name=%q storage_path=%q err=%q", dirPath, originalName, rel, err)
		return nil, err
	}

	file := &model.File{
		ID:          uuid.NewString(),
		Name:        name,
		Path:        dirPath,
		StoragePath: filepath.ToSlash(rel),
		IsDir:       true,
		Status:      model.FileStatusReady,
	}
	if err := s.store.CreateFile(ctx, file); err != nil {
		log.Printf("level=error component=file event=create_folder_store_failed file_id=%s path=%q name=%q err=%q", file.ID, dirPath, name, err)
		return nil, err
	}
	log.Printf("level=info component=file event=create_folder_complete file_id=%s path=%q name=%q storage_path=%q duration_ms=%d", file.ID, dirPath, name, file.StoragePath, time.Since(started).Milliseconds())
	return file, nil
}

func (s *FileService) Get(ctx context.Context, id string) (*model.File, error) {
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	if meta, err := s.store.GetMetadata(ctx, id); err == nil {
		file.Metadata = meta
	}
	return file, nil
}

func (s *FileService) DownloadPath(ctx context.Context, id string) (*model.File, string, error) {
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, "", err
	}
	if file.IsDir {
		return nil, "", errors.New("folders cannot be downloaded")
	}
	return file, s.absStoragePath(file.StoragePath), nil
}

func (s *FileService) ThumbnailPath(ctx context.Context, id string) (string, error) {
	if _, err := s.store.GetFile(ctx, id); err != nil {
		return "", err
	}
	meta, err := s.store.GetMetadata(ctx, id)
	if err != nil {
		return "", err
	}
	if meta.ThumbnailPath == nil || *meta.ThumbnailPath == "" {
		return "", store.ErrNotFound
	}
	if filepath.IsAbs(*meta.ThumbnailPath) {
		return *meta.ThumbnailPath, nil
	}
	return filepath.Join(s.cfg.Storage.ThumbnailDir, filepath.FromSlash(*meta.ThumbnailPath)), nil
}

func (s *FileService) Metadata(ctx context.Context, id string) (*model.FileMetadata, error) {
	if _, err := s.store.GetFile(ctx, id); err != nil {
		return nil, err
	}
	return s.store.GetMetadata(ctx, id)
}

func (s *FileService) RenameMove(ctx context.Context, id, newName, newPath string) (*model.File, error) {
	started := time.Now()
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		log.Printf("level=error component=file event=rename_move_lookup_failed file_id=%s err=%q", id, err)
		return nil, err
	}
	oldName := file.Name
	oldPath := file.Path
	oldStoragePath := file.StoragePath
	if newName == "" {
		newName = oldName
	}
	newName = SafeName(newName)
	if newName == "" {
		return nil, errors.New("file name is required")
	}
	if newPath == "" {
		newPath = oldPath
	} else {
		newPath = CleanVirtualPath(newPath)
	}

	if oldName == newName && oldPath == newPath {
		log.Printf("level=info component=file event=rename_move_noop file_id=%s path=%q name=%q duration_ms=%d", file.ID, oldPath, oldName, time.Since(started).Milliseconds())
		return file, nil
	}

	var oldVirtual string
	var newVirtual string
	if file.IsDir {
		oldVirtual = CleanVirtualPath(path.Join(oldPath, oldName))
		newVirtual = CleanVirtualPath(path.Join(newPath, newName))
		if strings.HasPrefix(newVirtual+"/", oldVirtual+"/") {
			err := fmt.Errorf("%w: cannot move a directory into itself", ErrPathConflict)
			log.Printf("level=warn component=file event=rename_move_conflict file_id=%s reason=self_descendant old_virtual=%q new_virtual=%q", id, oldVirtual, newVirtual)
			return nil, err
		}
	}

	if oldPath != newPath || !strings.EqualFold(oldName, newName) {
		exists, err := s.store.ExistsAtPath(ctx, newPath, newName)
		if err != nil {
			log.Printf("level=error component=file event=rename_move_conflict_check_failed file_id=%s new_path=%q new_name=%q err=%q", id, newPath, newName, err)
			return nil, err
		}
		if exists {
			err := fmt.Errorf("%w: %q already exists at %q", ErrPathConflict, newName, newPath)
			log.Printf("level=warn component=file event=rename_move_conflict file_id=%s reason=target_exists new_path=%q new_name=%q", id, newPath, newName)
			return nil, err
		}
	}

	oldAbs := s.absStoragePath(file.StoragePath)
	var newRel string
	if file.IsDir {
		newRel = strings.TrimPrefix(path.Join(newPath, newName), "/")
	} else {
		newRel = strings.TrimPrefix(path.Join(newPath, path.Base(file.StoragePath)), "/")
	}
	newAbs := s.absStoragePath(newRel)
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		log.Printf("level=error component=file event=rename_move_mkdir_failed file_id=%s new_path=%q new_name=%q err=%q", id, newPath, newName, err)
		return nil, err
	}
	if oldAbs != newAbs {
		if err := os.Rename(oldAbs, newAbs); err != nil {
			log.Printf("level=error component=file event=rename_move_fs_failed file_id=%s old_storage_path=%q new_storage_path=%q err=%q", id, oldStoragePath, filepath.ToSlash(newRel), err)
			return nil, err
		}
	}
	file.Name = newName
	file.Path = newPath
	file.StoragePath = filepath.ToSlash(newRel)
	if err := s.store.UpdateFileLocation(ctx, file); err != nil {
		log.Printf("level=error component=file event=rename_move_store_failed file_id=%s err=%q", id, err)
		return nil, err
	}
	childrenUpdated := 0
	if file.IsDir {
		oldStoragePrefix := strings.TrimPrefix(oldVirtual, "/")
		newStoragePrefix := strings.TrimPrefix(newVirtual, "/")
		children, err := s.store.ListDescendants(ctx, oldVirtual)
		if err != nil {
			log.Printf("level=error component=file event=rename_move_descendants_failed file_id=%s old_virtual=%q err=%q", id, oldVirtual, err)
			return nil, err
		}
		for _, child := range children {
			newChildPath := CleanVirtualPath(newVirtual + strings.TrimPrefix(child.Path, oldVirtual))
			newChildStorage := filepath.ToSlash(newStoragePrefix + strings.TrimPrefix(child.StoragePath, oldStoragePrefix))
			if err := s.store.UpdateFilePath(ctx, child.ID, newChildPath, newChildStorage); err != nil {
				log.Printf("level=error component=file event=rename_move_child_failed file_id=%s child_id=%s err=%q", id, child.ID, err)
				return nil, err
			}
			childrenUpdated++
		}
	}
	log.Printf("level=info component=file event=rename_move_complete file_id=%s old_path=%q new_path=%q old_name=%q new_name=%q old_storage_path=%q new_storage_path=%q is_dir=%t children_updated=%d duration_ms=%d",
		file.ID, oldPath, file.Path, oldName, file.Name, oldStoragePath, file.StoragePath, file.IsDir, childrenUpdated, time.Since(started).Milliseconds())
	return file, nil
}

func (s *FileService) Delete(ctx context.Context, id string) error {
	return s.SoftDelete(ctx, id)
}

func (s *FileService) RegisterUploadedFile(ctx context.Context, name, destPath, storagePath, mimeType string, size int64, chunkCount int) (*model.File, error) {
	started := time.Now()
	file := &model.File{
		ID:          uuid.NewString(),
		Name:        SafeName(name),
		Path:        CleanVirtualPath(destPath),
		StoragePath: filepath.ToSlash(storagePath),
		Size:        size,
		MimeType:    mimeType,
		Status:      model.FileStatusUploaded,
		ChunkCount:  chunkCount,
	}
	if err := s.store.CreateFile(ctx, file); err != nil {
		log.Printf("level=error component=file event=register_uploaded_failed file_id=%s name=%q path=%q storage_path=%q err=%q", file.ID, file.Name, file.Path, file.StoragePath, err)
		return nil, err
	}
	log.Printf("level=info component=file event=register_uploaded_complete file_id=%s name=%q path=%q storage_path=%q size=%d mime_type=%q upload_chunks=%d duration_ms=%d",
		file.ID, file.Name, file.Path, file.StoragePath, file.Size, file.MimeType, file.ChunkCount, time.Since(started).Milliseconds())
	return file, nil
}

func (s *FileService) BuildStorageRel(fileID, destPath, fileName string) string {
	ext := filepath.Ext(fileName)
	base := strings.TrimSuffix(SafeName(fileName), ext)
	if base == "" {
		base = "file"
	}
	name := fmt.Sprintf("%s-%s%s", fileID, base, ext)
	return filepath.ToSlash(path.Join(strings.TrimPrefix(CleanVirtualPath(destPath), "/"), name))
}

func (s *FileService) absStoragePath(rel string) string {
	return filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(rel))
}

func CleanVirtualPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		return "/"
	}
	cleaned := path.Clean("/" + strings.TrimPrefix(value, "/"))
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func SafeName(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = path.Base(value)
	value = strings.Trim(value, ". ")
	replacer := strings.NewReplacer("/", "_", "\x00", "", ":", "-", "*", "-", "?", "", "\"", "'", "<", "(", ">", ")", "|", "-")
	value = replacer.Replace(value)
	if value == "." || value == ".." {
		return ""
	}
	return value
}
