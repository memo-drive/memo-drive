package service

import (
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

const (
	maxRecoveredTaskRetries = 2
	maxJanitorRemovals      = 100
)

// Reconciler performs background maintenance: recovering stuck pipeline tasks,
// cleaning up orphaned files, and purging expired trash entries.
type Reconciler struct {
	cfg       *config.Config
	store     *store.Store
	pipeline  *PipelineService
	files     *FileService
	mutations *FileMutationService
}

// SweepStats tracks the results of a periodic reconciler sweep.
type SweepStats struct {
	TasksRecovered      int
	TasksFailed         int
	FilesFailed         int
	ThumbnailsRemoved   int
	WebDAVTempRemoved   int
	StorageMoved        int
	TrashPurged         int
	MutationsRemoved    int
	StagingRemoved      int
	FileVersionsRemoved int
}

// NewReconciler creates a new Reconciler.
func NewReconciler(cfg *config.Config, store *store.Store, pipeline *PipelineService, files *FileService) *Reconciler {
	return &Reconciler{
		cfg:       cfg,
		store:     store,
		pipeline:  pipeline,
		files:     files,
		mutations: NewFileMutationService(cfg, store),
	}
}

func (r *Reconciler) RecoverOnBoot(ctx context.Context) error {
	started := time.Now()
	if err := r.mutations.Recover(ctx); err != nil {
		return err
	}
	if err := r.recoverFolderCopies(ctx); err != nil {
		return err
	}
	uploadsFailed, err := r.store.FailMergingUploadSessions(ctx)
	if err != nil {
		return err
	}
	stats, err := r.recoverTasks(ctx, time.Now().Add(time.Second))
	if err != nil {
		return err
	}
	filesFailed, err := r.failProcessingFilesWithoutTasks(ctx, "processing file has no active task after boot")
	if err != nil {
		return err
	}
	stats.FilesFailed += filesFailed
	log.Printf("level=info component=reconciler event=recover_complete tasks_recovered=%d tasks_failed=%d files_failed=%d uploads_failed=%d duration_ms=%d",
		stats.TasksRecovered, stats.TasksFailed, stats.FilesFailed, uploadsFailed, time.Since(started).Milliseconds())
	return nil
}

func (r *Reconciler) recoverFolderCopies(ctx context.Context) error {
	operations, err := r.store.ListRunningFileCopyOperations(ctx)
	if err != nil {
		return err
	}
	for i := range operations {
		operation := &operations[i]
		if operation.RootFileID != "" {
			if err := r.files.SoftDelete(ctx, operation.RootFileID); err != nil && !errors.Is(err, store.ErrNotFound) {
				return err
			}
			if _, err := r.store.GetFileIncludeDeleted(ctx, operation.RootFileID); err == nil {
				if err := r.files.Purge(ctx, operation.RootFileID); err != nil {
					return err
				}
			} else if !errors.Is(err, store.ErrNotFound) {
				return err
			}
		}
		if err := r.store.UpdateFileCopyOperationState(
			ctx,
			operation.ID,
			model.FileCopyOperationStateFailed,
			"rolled back during startup recovery",
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *Reconciler) PeriodicSweep(ctx context.Context) error {
	started := time.Now()
	log.Printf("level=info component=janitor event=sweep_begin")
	stats := SweepStats{}
	cutoff := time.Now().Add(-r.cfg.Janitor.MaxTaskAge)
	taskStats, err := r.failStuckTasks(ctx, cutoff)
	if err != nil {
		return err
	}
	stats.TasksFailed += taskStats.TasksFailed
	filesFailed, err := r.failProcessingFilesWithoutTasks(ctx, "processing file has no active task")
	if err != nil {
		return err
	}
	stats.FilesFailed += filesFailed
	thumbs, err := r.SweepThumbnails(ctx)
	if err != nil {
		log.Printf("level=warn component=janitor event=thumbnail_sweep_failed err=%q", err)
	} else {
		stats.ThumbnailsRemoved = thumbs
	}
	webDAVTempRemoved, err := r.SweepWebDAVTemp(ctx)
	if err != nil {
		log.Printf("level=warn component=janitor event=webdav_temp_sweep_failed err=%q", err)
	} else {
		stats.WebDAVTempRemoved = webDAVTempRemoved
	}
	if r.cfg.Janitor.SweepStorageEnabled {
		moved, err := r.SweepStorage(ctx)
		if err != nil {
			log.Printf("level=warn component=janitor event=storage_sweep_failed err=%q", err)
		} else {
			stats.StorageMoved = moved
		}
	}
	mutationTTL := r.cfg.Storage.UploadTTL
	if mutationTTL == 0 {
		mutationTTL = time.Hour
	}
	mutationsRemoved, err := r.mutations.SweepTerminal(ctx, time.Now().Add(-mutationTTL), maxJanitorRemovals)
	if err != nil {
		log.Printf("level=warn component=janitor event=file_mutation_sweep_failed err=%q", err)
	} else {
		stats.MutationsRemoved = mutationsRemoved
	}
	stagingRemoved, err := r.mutations.SweepOrphanStaging(ctx, time.Now().Add(-mutationTTL), maxJanitorRemovals)
	if err != nil {
		log.Printf("level=warn component=janitor event=orphan_staging_sweep_failed err=%q", err)
	} else {
		stats.StagingRemoved = stagingRemoved
	}
	trashPurged, err := r.SweepTrash(ctx)
	if err != nil {
		log.Printf("level=warn component=janitor event=trash_sweep_failed err=%q", err)
	} else {
		stats.TrashPurged = trashPurged
	}
	versionsRemoved, err := r.SweepFileVersions(ctx)
	if err != nil {
		log.Printf("level=warn component=janitor event=file_version_sweep_failed err=%q", err)
	} else {
		stats.FileVersionsRemoved = versionsRemoved
	}
	log.Printf("level=info component=janitor event=sweep_complete tasks_recovered=%d tasks_failed=%d files_failed=%d thumbnails_removed=%d webdav_temp_removed=%d storage_moved=%d mutations_removed=%d staging_removed=%d trash_purged=%d file_versions_removed=%d duration_ms=%d",
		stats.TasksRecovered, stats.TasksFailed, stats.FilesFailed, stats.ThumbnailsRemoved, stats.WebDAVTempRemoved, stats.StorageMoved, stats.MutationsRemoved, stats.StagingRemoved, stats.TrashPurged, stats.FileVersionsRemoved, time.Since(started).Milliseconds())
	return nil
}

func (r *Reconciler) SweepFileVersions(ctx context.Context) (int, error) {
	if r.cfg == nil || r.files == nil || r.cfg.FileVersion.MaxCount <= 0 {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(r.cfg.FileVersion.RetentionDays) * 24 * time.Hour)
	versions, err := r.store.ListFileVersionsForRetention(ctx, r.cfg.FileVersion.MaxCount, cutoff, maxJanitorRemovals)
	if err != nil {
		return 0, err
	}
	removed := 0
	for i := range versions {
		if ctx.Err() != nil {
			return removed, ctx.Err()
		}
		if err := r.files.DeleteVersion(ctx, versions[i].FileID, versions[i].ID); err != nil {
			log.Printf("level=warn component=janitor event=file_version_remove_failed file_id=%s version_id=%s err=%q", versions[i].FileID, versions[i].ID, err)
			continue
		}
		removed++
	}
	return removed, nil
}

func (r *Reconciler) recoverTasks(ctx context.Context, olderThan time.Time) (SweepStats, error) {
	var stats SweepStats
	tasks, err := r.store.ListStuckTasks(ctx, olderThan)
	if err != nil {
		return stats, err
	}
	for _, task := range tasks {
		_, err := r.store.GetFile(ctx, task.FileID)
		if err != nil {
			msg := "stuck task file is missing"
			_ = r.store.MarkTaskFailed(ctx, task.ID, msg)
			stats.TasksFailed++
			log.Printf("level=warn component=reconciler event=task_failed_missing_file task_id=%s file_id=%s err=%q", task.ID, task.FileID, err)
			continue
		}
		if task.RetryCount >= maxRecoveredTaskRetries {
			msg := "task exceeded restart recovery retry limit"
			_ = r.store.MarkTaskFailed(ctx, task.ID, msg)
			_ = r.store.UpdateFileStatus(ctx, task.FileID, model.FileStatusFailed)
			stats.TasksFailed++
			log.Printf("level=warn component=reconciler event=task_retry_exhausted task_id=%s file_id=%s retry_count=%d", task.ID, task.FileID, task.RetryCount)
			continue
		}
		if r.pipeline == nil {
			msg := "task cannot be requeued because pipeline is unavailable"
			_ = r.store.MarkTaskFailed(ctx, task.ID, msg)
			_ = r.store.UpdateFileStatus(ctx, task.FileID, model.FileStatusFailed)
			stats.TasksFailed++
			continue
		}
		recoveryReason := "task replaced by a new attempt during startup recovery"
		if err := r.store.MarkTaskFailed(ctx, task.ID, recoveryReason); err != nil {
			return stats, err
		}
		retry, err := r.pipeline.RetryTask(ctx, task.ID)
		if err != nil {
			_ = r.store.UpdateFileStatus(ctx, task.FileID, model.FileStatusFailed)
			stats.TasksFailed++
			log.Printf("level=warn component=reconciler event=requeue_failed task_id=%s file_id=%s err=%q", task.ID, task.FileID, err)
			continue
		}
		stats.TasksRecovered++
		log.Printf("level=info component=reconciler event=requeued_task old_task_id=%s new_task_id=%s file_id=%s retry_count=%d", task.ID, retry.ID, task.FileID, retry.RetryCount)
	}
	return stats, nil
}

func (r *Reconciler) failStuckTasks(ctx context.Context, olderThan time.Time) (SweepStats, error) {
	var stats SweepStats
	tasks, err := r.store.ListStuckTasks(ctx, olderThan)
	if err != nil {
		return stats, err
	}
	for _, task := range tasks {
		msg := "task stuck for longer than JANITOR_MAX_TASK_AGE_SECONDS"
		_ = r.store.MarkTaskFailed(ctx, task.ID, msg)
		_ = r.store.UpdateFileStatus(ctx, task.FileID, model.FileStatusFailed)
		stats.TasksFailed++
		log.Printf("level=warn component=janitor event=stuck_task_failed task_id=%s file_id=%s updated_at=%s", task.ID, task.FileID, task.UpdatedAt.Format(time.RFC3339))
	}
	return stats, nil
}

func (r *Reconciler) failProcessingFilesWithoutTasks(ctx context.Context, reason string) (int, error) {
	files, err := r.store.ListFilesByStatus(ctx, model.FileStatusProcessing)
	if err != nil {
		return 0, err
	}
	failed := 0
	for _, file := range files {
		active, err := r.store.HasActiveTaskForFile(ctx, file.ID)
		if err != nil {
			return failed, err
		}
		if active {
			continue
		}
		_ = r.store.UpdateFileStatus(ctx, file.ID, model.FileStatusFailed)
		failed++
		log.Printf("level=warn component=reconciler event=orphan_processing_file_failed file_id=%s file_name=%q reason=%q", file.ID, file.Name, reason)
	}
	return failed, nil
}

func (r *Reconciler) SweepThumbnails(ctx context.Context) (int, error) {
	if r.cfg == nil || r.cfg.Storage.ThumbnailDir == "" {
		return 0, nil
	}
	entries, err := os.ReadDir(r.cfg.Storage.ThumbnailDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		if removed >= maxJanitorRemovals || ctx.Err() != nil {
			break
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jpg" {
			continue
		}
		fileID := entry.Name()[:len(entry.Name())-len(filepath.Ext(entry.Name()))]
		exists, err := r.store.FileExists(ctx, fileID)
		if err != nil {
			return removed, err
		}
		if exists {
			continue
		}
		path := filepath.Join(r.cfg.Storage.ThumbnailDir, entry.Name())
		if err := os.Remove(path); err != nil {
			log.Printf("level=warn component=janitor event=thumbnail_orphan_remove_failed file=%q err=%q", path, err)
			continue
		}
		removed++
		log.Printf("level=info component=janitor event=thumbnail_orphan_removed file=%q", path)
	}
	return removed, ctx.Err()
}

func (r *Reconciler) SweepWebDAVTemp(ctx context.Context) (int, error) {
	if r.cfg == nil || r.cfg.Storage.TempDir == "" {
		return 0, nil
	}
	ttl := r.cfg.Storage.UploadTTL
	if ttl <= 0 {
		ttl = time.Hour
	}
	tempDir := filepath.Join(r.cfg.Storage.TempDir, "webdav")
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-ttl)
	removed := 0
	for _, entry := range entries {
		if removed >= maxJanitorRemovals || ctx.Err() != nil {
			break
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".upload" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			log.Printf("level=warn component=janitor event=webdav_temp_stat_failed file=%q err=%q", filepath.Join(tempDir, entry.Name()), err)
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		path := filepath.Join(tempDir, entry.Name())
		if err := os.Remove(path); err != nil {
			log.Printf("level=warn component=janitor event=webdav_temp_remove_failed file=%q err=%q", path, err)
			continue
		}
		removed++
		log.Printf("level=info component=janitor event=webdav_temp_removed file=%q", path)
	}
	return removed, ctx.Err()
}

func (r *Reconciler) SweepStorage(ctx context.Context) (int, error) {
	known, err := r.store.ListStoragePaths(ctx)
	if err != nil {
		return 0, err
	}
	root := r.cfg.Storage.Root
	trash := filepath.Join(root, ".trash")
	staging := filepath.Join(root, ".staging")
	moved := 0
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if moved >= maxJanitorRemovals || ctx.Err() != nil {
			return filepath.SkipAll
		}
		if d.IsDir() {
			if path == trash || path == staging {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if _, ok := known[rel]; ok {
			return nil
		}
		dest := filepath.Join(trash, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.Rename(path, dest); err != nil {
			log.Printf("level=warn component=janitor event=storage_orphan_move_failed file=%q dest=%q err=%q", path, dest, err)
			return nil
		}
		moved++
		log.Printf("level=info component=janitor event=storage_orphan_moved file=%q dest=%q", path, dest)
		return nil
	})
	if err != nil {
		return moved, err
	}
	return moved, ctx.Err()
}

func (r *Reconciler) SweepTrash(ctx context.Context) (int, error) {
	if r.cfg == nil || !r.cfg.Trash.AutoPurgeEnabled || r.files == nil {
		return 0, nil
	}
	cutoff := time.Now().Add(-time.Duration(r.cfg.Trash.RetentionDays) * 24 * time.Hour)
	expired, err := r.store.ListExpiredTrashed(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	purged := 0
	for _, file := range expired {
		if ctx.Err() != nil {
			return purged, ctx.Err()
		}
		if err := r.files.Purge(ctx, file.ID); err != nil {
			log.Printf("level=warn component=janitor event=trash_purge_failed file_id=%s err=%q", file.ID, err)
			continue
		}
		purged++
	}
	log.Printf("level=info component=janitor event=trash_purge_complete purged=%d cutoff=%s", purged, cutoff.Format(time.RFC3339))
	return purged, nil
}
