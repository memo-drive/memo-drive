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

func (s *FileService) SoftDelete(ctx context.Context, id string) error {
	started := time.Now()
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		log.Printf("level=error component=file event=soft_delete_lookup_failed file_id=%s err=%q", id, err)
		return err
	}
	childrenDeleted := 0
	if file.IsDir {
		oldVirtual := CleanVirtualPath(path.Join(file.Path, file.Name))
		children, err := s.store.ListDescendants(ctx, oldVirtual)
		if err != nil {
			log.Printf("level=error component=file event=soft_delete_descendants_failed file_id=%s old_virtual=%q err=%q", id, oldVirtual, err)
			return err
		}
		for i := len(children) - 1; i >= 0; i-- {
			if err := s.store.SoftDeleteFile(ctx, children[i].ID, file.ID); err != nil {
				log.Printf("level=warn component=file event=soft_delete_child_failed file_id=%s child_id=%s err=%q", id, children[i].ID, err)
				continue
			}
			childrenDeleted++
		}
	}
	if err := s.store.SoftDeleteFile(ctx, id, id); err != nil {
		log.Printf("level=error component=file event=soft_delete_store_failed file_id=%s err=%q", id, err)
		return err
	}
	log.Printf("level=info component=file event=soft_delete_complete file_id=%s name=%q storage_path=%q is_dir=%t children_deleted=%d duration_ms=%d",
		id, file.Name, file.StoragePath, file.IsDir, childrenDeleted, time.Since(started).Milliseconds())
	return nil
}

func (s *FileService) Restore(ctx context.Context, id string) (*model.File, error) {
	started := time.Now()
	file, err := s.store.GetFileIncludeDeleted(ctx, id)
	if err != nil {
		log.Printf("level=error component=file event=restore_lookup_failed file_id=%s err=%q", id, err)
		return nil, err
	}
	if file.DeletedAt == nil {
		return file, nil
	}

	fallbackPath, fallbackName := originalLocation(file)
	finalName, err := s.availableRestoreName(ctx, fallbackPath, fallbackName)
	if err != nil {
		log.Printf("level=error component=file event=restore_conflict_check_failed file_id=%s path=%q name=%q err=%q", id, fallbackPath, fallbackName, err)
		return nil, err
	}

	if err := s.store.RestoreFile(ctx, id, fallbackPath, finalName); err != nil {
		log.Printf("level=error component=file event=restore_store_failed file_id=%s path=%q name=%q err=%q", id, fallbackPath, finalName, err)
		return nil, err
	}
	childrenRestored := 0
	if file.IsDir {
		oldVirtual := CleanVirtualPath(path.Join(fallbackPath, fallbackName))
		newVirtual := CleanVirtualPath(path.Join(fallbackPath, finalName))
		childrenRestored, err = s.restoreTrashedDescendants(ctx, id, oldVirtual, newVirtual)
		if err != nil {
			return nil, err
		}
	}
	restored, err := s.store.GetFile(ctx, id)
	if err != nil {
		return nil, err
	}
	log.Printf("level=info component=file event=restore_complete file_id=%s original_path=%q restored_path=%q restored_name=%q is_dir=%t children_restored=%d duration_ms=%d",
		id, fallbackPath, restored.Path, restored.Name, restored.IsDir, childrenRestored, time.Since(started).Milliseconds())
	return restored, nil
}

func (s *FileService) Purge(ctx context.Context, id string) error {
	started := time.Now()
	file, err := s.store.GetFileIncludeDeleted(ctx, id)
	if err != nil {
		log.Printf("level=error component=file event=purge_lookup_failed file_id=%s err=%q", id, err)
		return err
	}
	if file.DeletedAt == nil {
		return fmt.Errorf("%w: soft delete first", ErrNotInTrash)
	}

	childrenPurged := 0
	if file.IsDir {
		originalPath, originalName := originalLocation(file)
		oldVirtual := CleanVirtualPath(path.Join(originalPath, originalName))
		children, err := s.store.ListTrashedDescendants(ctx, oldVirtual)
		if err != nil {
			log.Printf("level=error component=file event=purge_descendants_failed file_id=%s old_virtual=%q err=%q", id, oldVirtual, err)
			return err
		}
		for i := len(children) - 1; i >= 0; i-- {
			if err := s.Purge(ctx, children[i].ID); err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				log.Printf("level=warn component=file event=purge_child_failed file_id=%s child_id=%s err=%q", id, children[i].ID, err)
				continue
			}
			childrenPurged++
		}
	}

	if err := os.RemoveAll(s.absStoragePath(file.StoragePath)); err != nil {
		log.Printf("level=warn component=file event=purge_fs_failed file_id=%s storage_path=%q err=%q", id, file.StoragePath, err)
	}
	if !file.IsDir && file.ChunkCount > 0 && s.vectorDB != nil {
		ids := vectordb.ChunkIDs(file.ID, file.ChunkCount)
		if err := s.vectorDB.Delete(ctx, vectordb.DefaultCollection, ids); err != nil {
			log.Printf("level=warn component=file event=vector_cleanup_failed file_id=%s chunk_count=%d err=%q", id, file.ChunkCount, err)
		} else {
			log.Printf("level=info component=file event=vector_cleanup_complete file_id=%s chunks_deleted=%d", id, file.ChunkCount)
		}
	}
	if !file.IsDir {
		if err := s.store.DeleteChunksByFileID(ctx, file.ID); err != nil {
			log.Printf("level=warn component=file event=chunk_cleanup_failed file_id=%s err=%q", id, err)
		} else {
			log.Printf("level=info component=file event=chunk_cleanup_complete file_id=%s", id)
		}
	}
	if err := s.store.PurgeFile(ctx, id); err != nil {
		log.Printf("level=error component=file event=purge_store_failed file_id=%s err=%q", id, err)
		return err
	}
	log.Printf("level=info component=file event=purge_complete file_id=%s name=%q storage_path=%q is_dir=%t children_purged=%d duration_ms=%d",
		id, displayOriginalName(file), file.StoragePath, file.IsDir, childrenPurged, time.Since(started).Milliseconds())
	return nil
}

func (s *FileService) ListTrashed(ctx context.Context, limit int) ([]model.File, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.store.ListTrashed(ctx, limit)
}

func (s *FileService) EmptyTrash(ctx context.Context) (int, error) {
	items, err := s.store.ListTrashed(ctx, 5000)
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, file := range items {
		if err := s.Purge(ctx, file.ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			log.Printf("level=warn component=file event=empty_trash_item_failed file_id=%s err=%q", file.ID, err)
			continue
		}
		purged++
	}
	log.Printf("level=info component=file event=empty_trash_complete purged=%d", purged)
	return purged, nil
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

func originalLocation(file *model.File) (string, string) {
	fallbackPath := "/"
	if file.OriginalPath != nil {
		fallbackPath = *file.OriginalPath
	} else if file.Path != "" && file.Path != "/.trash" {
		fallbackPath = file.Path
	}
	fallbackPath = CleanVirtualPath(fallbackPath)

	fallbackName := file.Name
	if file.OriginalName != nil {
		fallbackName = *file.OriginalName
	}
	fallbackName = SafeName(fallbackName)
	if fallbackName == "" {
		fallbackName = file.ID
	}
	return fallbackPath, fallbackName
}

func restoredName(name, suffix string) string {
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	if base == "" {
		base = "file"
	}
	return fmt.Sprintf("%s (restored%s)%s", base, suffix, ext)
}

func (s *FileService) availableRestoreName(ctx context.Context, dirPath, name string) (string, error) {
	finalName := name
	for i := 0; i < 100; i++ {
		exists, err := s.store.ExistsAtPath(ctx, dirPath, finalName)
		if err != nil {
			return "", err
		}
		if !exists {
			return finalName, nil
		}
		if i == 0 {
			finalName = restoredName(name, "")
		} else {
			finalName = restoredName(name, fmt.Sprintf(" %d", i))
		}
	}
	return "", fmt.Errorf("%w: restore target has too many conflicts", ErrPathConflict)
}

func (s *FileService) restoreTrashedDescendants(ctx context.Context, rootID, oldRoot, newRoot string) (int, error) {
	children, err := s.store.ListTrashedDescendants(ctx, oldRoot)
	if err != nil {
		log.Printf("level=error component=file event=restore_descendants_failed file_id=%s old_virtual=%q err=%q", rootID, oldRoot, err)
		return 0, err
	}
	pathMap := map[string]string{
		CleanVirtualPath(oldRoot): CleanVirtualPath(newRoot),
	}
	restored := 0
	for _, child := range children {
		childPath, childName := originalLocation(&child)
		mappedPath := mappedRestorePath(childPath, pathMap)
		finalName, err := s.availableRestoreName(ctx, mappedPath, childName)
		if err != nil {
			log.Printf("level=error component=file event=restore_child_conflict_failed file_id=%s child_id=%s path=%q name=%q err=%q", rootID, child.ID, mappedPath, childName, err)
			return restored, err
		}
		if err := s.store.RestoreFile(ctx, child.ID, mappedPath, finalName); err != nil {
			log.Printf("level=error component=file event=restore_child_failed file_id=%s child_id=%s path=%q name=%q err=%q", rootID, child.ID, mappedPath, finalName, err)
			return restored, err
		}
		restored++
		if child.IsDir {
			oldChildVirtual := CleanVirtualPath(path.Join(childPath, childName))
			newChildVirtual := CleanVirtualPath(path.Join(mappedPath, finalName))
			pathMap[oldChildVirtual] = newChildVirtual
		}
	}
	return restored, nil
}

func mappedRestorePath(originalPath string, pathMap map[string]string) string {
	originalPath = CleanVirtualPath(originalPath)
	bestOld := ""
	bestNew := originalPath
	for oldPath, newPath := range pathMap {
		oldPath = CleanVirtualPath(oldPath)
		if originalPath != oldPath && !strings.HasPrefix(originalPath, oldPath+"/") {
			continue
		}
		if len(oldPath) <= len(bestOld) {
			continue
		}
		bestOld = oldPath
		bestNew = CleanVirtualPath(newPath + strings.TrimPrefix(originalPath, oldPath))
	}
	return bestNew
}

func displayOriginalName(file *model.File) string {
	if file.OriginalName != nil && *file.OriginalName != "" {
		return *file.OriginalName
	}
	return file.Name
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
