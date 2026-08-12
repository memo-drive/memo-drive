package admin

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/maintenance"
)

type RestoreOptions struct {
	BackupPath string
	TargetRoot string
	TargetDB   string
	Force      bool
}

type RestoreSummary struct {
	Command          string `json:"command"`
	Success          bool   `json:"success"`
	TargetRoot       string `json:"target_root"`
	TargetDB         string `json:"target_db"`
	TargetThumbnails string `json:"target_thumbnails"`
	RestoredFiles    int    `json:"restored_files"`
}

func (m *Manager) Restore(ctx context.Context, options RestoreOptions) (RestoreSummary, error) {
	backupPath, err := filepath.Abs(options.BackupPath)
	if err != nil {
		return RestoreSummary{}, fmt.Errorf("resolve backup path: %w", err)
	}
	targetRoot, err := filepath.Abs(options.TargetRoot)
	if err != nil {
		return RestoreSummary{}, fmt.Errorf("resolve target root: %w", err)
	}
	targetDB, err := filepath.Abs(options.TargetDB)
	if err != nil {
		return RestoreSummary{}, fmt.Errorf("resolve target database: %w", err)
	}
	targetThumbnails := filepath.Join(filepath.Dir(targetRoot), "thumbnails")

	if _, err := m.Verify(ctx, backupPath, 0); err != nil {
		return RestoreSummary{}, fmt.Errorf("verify backup before restore: %w", err)
	}
	manifest, err := readManifest(filepath.Join(backupPath, "manifest.json"))
	if err != nil {
		return RestoreSummary{}, err
	}
	writerLock, err := maintenance.AcquireWriterLock(targetDB)
	if err != nil {
		return RestoreSummary{}, err
	}
	defer writerLock.Close()

	if err := os.MkdirAll(filepath.Dir(targetRoot), 0o755); err != nil {
		return RestoreSummary{}, fmt.Errorf("create target root parent: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetDB), 0o755); err != nil {
		return RestoreSummary{}, fmt.Errorf("create target database parent: %w", err)
	}
	rootStage, err := os.MkdirTemp(filepath.Dir(targetRoot), ".memodrive-files-restore-*")
	if err != nil {
		return RestoreSummary{}, fmt.Errorf("create file restore staging: %w", err)
	}
	defer os.RemoveAll(rootStage)
	thumbnailStage, err := os.MkdirTemp(filepath.Dir(targetThumbnails), ".memodrive-thumbnails-restore-*")
	if err != nil {
		return RestoreSummary{}, fmt.Errorf("create thumbnail restore staging: %w", err)
	}
	defer os.RemoveAll(thumbnailStage)
	databaseStage, err := unusedTempPath(filepath.Dir(targetDB), ".memodrive-db-restore-*")
	if err != nil {
		return RestoreSummary{}, fmt.Errorf("create database restore staging: %w", err)
	}
	defer os.Remove(databaseStage)

	for _, file := range manifest.Files {
		source, err := safeJoin(filepath.Join(backupPath, "files"), file.StoragePath)
		if err != nil {
			return RestoreSummary{}, err
		}
		destination, err := safeJoin(rootStage, file.StoragePath)
		if err != nil {
			return RestoreSummary{}, err
		}
		if err := copyFile(source, destination); err != nil {
			return RestoreSummary{}, fmt.Errorf("restore file %s: %w", file.FileID, err)
		}
	}
	for _, version := range manifest.FileVersions {
		source, err := safeJoin(filepath.Join(backupPath, "files"), version.StoragePath)
		if err != nil {
			return RestoreSummary{}, err
		}
		destination, err := safeJoin(rootStage, version.StoragePath)
		if err != nil {
			return RestoreSummary{}, err
		}
		if err := copyFile(source, destination); err != nil {
			return RestoreSummary{}, fmt.Errorf("restore File Version %s: %w", version.VersionID, err)
		}
	}
	for _, thumbnail := range manifest.Thumbnails {
		source, err := safeJoin(filepath.Join(backupPath, "thumbnails"), thumbnail.Path)
		if err != nil {
			return RestoreSummary{}, err
		}
		destination, err := safeJoin(thumbnailStage, thumbnail.Path)
		if err != nil {
			return RestoreSummary{}, err
		}
		if err := copyFile(source, destination); err != nil {
			return RestoreSummary{}, fmt.Errorf("restore thumbnail for file %s: %w", thumbnail.FileID, err)
		}
	}
	if err := copyFile(filepath.Join(backupPath, "db", "memodrive.db"), databaseStage); err != nil {
		return RestoreSummary{}, fmt.Errorf("restore database snapshot: %w", err)
	}
	if err := normalizeRestoredWork(ctx, databaseStage); err != nil {
		return RestoreSummary{}, err
	}
	stagedConfig := &config.Config{Storage: config.StorageConfig{
		Root:         rootStage,
		DBPath:       databaseStage,
		ThumbnailDir: thumbnailStage,
	}}
	if _, err := New(stagedConfig, m.appVersion).Integrity(ctx); err != nil {
		return RestoreSummary{}, fmt.Errorf("verify restored staging: %w", err)
	}

	items := []restorePublishItem{
		{stage: rootStage, target: targetRoot},
		{stage: thumbnailStage, target: targetThumbnails},
		{stage: databaseStage, target: targetDB},
		{target: targetDB + "-wal"},
		{target: targetDB + "-shm"},
	}
	if err := publishRestore(items, options.Force); err != nil {
		return RestoreSummary{}, err
	}
	return RestoreSummary{
		Command:          "restore",
		Success:          true,
		TargetRoot:       targetRoot,
		TargetDB:         targetDB,
		TargetThumbnails: targetThumbnails,
		RestoredFiles:    manifest.FileCount,
	}, nil
}

func normalizeRestoredWork(ctx context.Context, databasePath string) error {
	db, err := sql.Open("sqlite3", databasePath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open restored database staging: %w", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `
UPDATE upload_sessions
SET status = 'failed'
WHERE status IN ('uploading', 'merging');
UPDATE tasks
SET status = 'pending', progress = 0, error = NULL, updated_at = CURRENT_TIMESTAMP
WHERE status = 'processing';`); err != nil {
		return fmt.Errorf("normalize restored work state: %w", err)
	}
	if err := sqliteIntegrityCheck(ctx, db); err != nil {
		return fmt.Errorf("check restored database staging: %w", err)
	}
	return nil
}

type restorePublishItem struct {
	stage     string
	target    string
	original  string
	published bool
}

func publishRestore(items []restorePublishItem, force bool) error {
	for index := range items {
		info, err := os.Stat(items[index].target)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			rollbackRestore(items)
			return fmt.Errorf("inspect restore target %q: %w", items[index].target, err)
		}
		if !force {
			if !info.IsDir() {
				rollbackRestore(items)
				return fmt.Errorf("restore target is not empty: %s; use --force to replace it", items[index].target)
			}
			entries, err := os.ReadDir(items[index].target)
			if err != nil {
				rollbackRestore(items)
				return fmt.Errorf("inspect restore target %q: %w", items[index].target, err)
			}
			if len(entries) != 0 {
				rollbackRestore(items)
				return fmt.Errorf("restore target is not empty: %s; use --force to replace it", items[index].target)
			}
			if err := os.Remove(items[index].target); err != nil {
				rollbackRestore(items)
				return fmt.Errorf("remove empty restore target %q: %w", items[index].target, err)
			}
			continue
		}
		original, err := unusedTempPath(filepath.Dir(items[index].target), ".memodrive-restore-original-*")
		if err != nil {
			rollbackRestore(items)
			return err
		}
		if err := os.Rename(items[index].target, original); err != nil {
			rollbackRestore(items)
			return fmt.Errorf("preserve restore target %q: %w", items[index].target, err)
		}
		items[index].original = original
	}

	for index := range items {
		if items[index].stage == "" {
			continue
		}
		if err := os.Rename(items[index].stage, items[index].target); err != nil {
			rollbackRestore(items)
			return fmt.Errorf("publish restored target %q: %w", items[index].target, err)
		}
		items[index].published = true
	}
	for _, item := range items {
		if item.original != "" {
			_ = os.RemoveAll(item.original)
		}
	}
	return nil
}

func rollbackRestore(items []restorePublishItem) {
	for index := len(items) - 1; index >= 0; index-- {
		if items[index].published {
			_ = os.RemoveAll(items[index].target)
		}
		if items[index].original != "" {
			_ = os.Rename(items[index].original, items[index].target)
		}
	}
}

func unusedTempPath(directory, pattern string) (string, error) {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(directory, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		return "", err
	}
	if err := os.Remove(path); err != nil {
		return "", err
	}
	return path, nil
}
