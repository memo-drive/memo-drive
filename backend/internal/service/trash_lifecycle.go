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

	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

func (s *FileService) SoftDelete(ctx context.Context, id string) error {
	started := time.Now()
	file, err := s.store.GetFile(ctx, id)
	if err != nil {
		log.Printf("level=error component=file event=soft_delete_lookup_failed file_id=%s err=%q", id, err)
		return err
	}

	childrenDeleted, err := s.softDeleteDescendants(ctx, file)
	if err != nil {
		return err
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
		return nil, fmt.Errorf("%w: restore requires a trash entry", ErrNotInTrash)
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
	childrenPurged, file, err := s.purgeTrashEntry(ctx, id)
	if err != nil {
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

func (s *FileService) softDeleteDescendants(ctx context.Context, file *model.File) (int, error) {
	if !file.IsDir {
		return 0, nil
	}
	oldVirtual := CleanVirtualPath(path.Join(file.Path, file.Name))
	children, err := s.store.ListDescendants(ctx, oldVirtual)
	if err != nil {
		log.Printf("level=error component=file event=soft_delete_descendants_failed file_id=%s old_virtual=%q err=%q", file.ID, oldVirtual, err)
		return 0, err
	}
	deleted := 0
	for i := len(children) - 1; i >= 0; i-- {
		if err := s.store.SoftDeleteFile(ctx, children[i].ID, file.ID); err != nil {
			log.Printf("level=warn component=file event=soft_delete_child_failed file_id=%s child_id=%s err=%q", file.ID, children[i].ID, err)
			continue
		}
		deleted++
	}
	return deleted, nil
}

func (s *FileService) purgeTrashEntry(ctx context.Context, id string) (int, *model.File, error) {
	file, err := s.store.GetFileIncludeDeleted(ctx, id)
	if err != nil {
		log.Printf("level=error component=file event=purge_lookup_failed file_id=%s err=%q", id, err)
		return 0, nil, err
	}
	if file.DeletedAt == nil {
		return 0, nil, fmt.Errorf("%w: soft delete first", ErrNotInTrash)
	}

	childrenPurged, err := s.purgeTrashedDescendants(ctx, file)
	if err != nil {
		return childrenPurged, nil, err
	}
	s.removeStoredObject(file)
	s.removeVectorChunks(ctx, file)
	s.removeChunkRows(ctx, file)
	if err := s.store.PurgeFile(ctx, id); err != nil {
		log.Printf("level=error component=file event=purge_store_failed file_id=%s err=%q", id, err)
		return childrenPurged, nil, err
	}
	return childrenPurged, file, nil
}

func (s *FileService) purgeTrashedDescendants(ctx context.Context, file *model.File) (int, error) {
	if !file.IsDir {
		return 0, nil
	}
	originalPath, originalName := originalLocation(file)
	oldVirtual := CleanVirtualPath(path.Join(originalPath, originalName))
	children, err := s.store.ListTrashedDescendants(ctx, oldVirtual)
	if err != nil {
		log.Printf("level=error component=file event=purge_descendants_failed file_id=%s old_virtual=%q err=%q", file.ID, oldVirtual, err)
		return 0, err
	}
	purged := 0
	for i := len(children) - 1; i >= 0; i-- {
		if _, _, err := s.purgeTrashEntry(ctx, children[i].ID); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				continue
			}
			log.Printf("level=warn component=file event=purge_child_failed file_id=%s child_id=%s err=%q", file.ID, children[i].ID, err)
			continue
		}
		purged++
	}
	return purged, nil
}

func (s *FileService) removeStoredObject(file *model.File) {
	if err := os.RemoveAll(s.absStoragePath(file.StoragePath)); err != nil {
		log.Printf("level=warn component=file event=purge_fs_failed file_id=%s storage_path=%q err=%q", file.ID, file.StoragePath, err)
	}
}

func (s *FileService) removeVectorChunks(ctx context.Context, file *model.File) {
	if file.IsDir || file.ChunkCount <= 0 || s.vectorDB == nil {
		return
	}
	ids := indexing.ChunkIDs(file.ID, file.ChunkCount)
	if err := s.vectorDB.Delete(ctx, vectordb.DefaultCollection, ids); err != nil {
		log.Printf("level=warn component=file event=vector_cleanup_failed file_id=%s chunk_count=%d err=%q", file.ID, file.ChunkCount, err)
		return
	}
	log.Printf("level=info component=file event=vector_cleanup_complete file_id=%s chunks_deleted=%d", file.ID, file.ChunkCount)
}

func (s *FileService) removeChunkRows(ctx context.Context, file *model.File) {
	if file.IsDir {
		return
	}
	if err := s.store.DeleteChunksByFileID(ctx, file.ID); err != nil {
		log.Printf("level=warn component=file event=chunk_cleanup_failed file_id=%s err=%q", file.ID, err)
		return
	}
	log.Printf("level=info component=file event=chunk_cleanup_complete file_id=%s", file.ID)
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
