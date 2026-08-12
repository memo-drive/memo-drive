package admin

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/memodrive/backend/internal/maintenance"
)

type IntegritySummary struct {
	Command           string `json:"command"`
	Success           bool   `json:"success"`
	CheckedFiles      int    `json:"checked_files"`
	CheckedVersions   int    `json:"checked_versions"`
	CheckedThumbnails int    `json:"checked_thumbnails"`
}

func (m *Manager) Integrity(ctx context.Context) (IntegritySummary, error) {
	writerLock, err := maintenance.AcquireWriterLock(m.cfg.Storage.DBPath)
	if err != nil {
		return IntegritySummary{}, err
	}
	defer writerLock.Close()

	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(m.cfg.Storage.DBPath)+"?mode=ro&_foreign_keys=on")
	if err != nil {
		return IntegritySummary{}, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	if err := sqliteIntegrityCheck(ctx, db); err != nil {
		return IntegritySummary{}, err
	}
	if err := checkDuplicateActiveTargetPaths(ctx, db); err != nil {
		return IntegritySummary{}, err
	}

	registered, checkedFiles, err := checkCurrentStorageFiles(ctx, db, m.cfg.Storage.Root)
	if err != nil {
		return IntegritySummary{}, err
	}
	checkedVersions, err := checkCurrentFileVersions(ctx, db, m.cfg.Storage.Root, registered)
	if err != nil {
		return IntegritySummary{}, err
	}
	if err := checkUnregisteredCurrentObjects(m.cfg.Storage.Root, registered); err != nil {
		return IntegritySummary{}, err
	}
	checkedThumbnails, err := checkCurrentThumbnails(ctx, db, m.cfg.Storage.ThumbnailDir)
	if err != nil {
		return IntegritySummary{}, err
	}
	return IntegritySummary{
		Command:           "integrity",
		Success:           true,
		CheckedFiles:      checkedFiles,
		CheckedVersions:   checkedVersions,
		CheckedThumbnails: checkedThumbnails,
	}, nil
}

func checkCurrentFileVersions(ctx context.Context, db *sql.DB, storageRoot string, registered map[string]struct{}) (int, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, storage_path, size, sha256 FROM file_versions ORDER BY id`)
	if err != nil {
		return 0, fmt.Errorf("list File Version storage references: %w", err)
	}
	defer rows.Close()
	checked := 0
	for rows.Next() {
		var versionID, storagePath string
		var expectedSize int64
		var expectedSHA sql.NullString
		if err := rows.Scan(&versionID, &storagePath, &expectedSize, &expectedSHA); err != nil {
			return checked, fmt.Errorf("scan File Version storage reference: %w", err)
		}
		objectPath, err := safeJoin(storageRoot, storagePath)
		if err != nil {
			return checked, fmt.Errorf("File Version %s: %w", versionID, err)
		}
		sha, size, err := fileDigest(objectPath)
		if err != nil {
			return checked, fmt.Errorf("File Version storage object missing: version_id=%s path=%q: %w", versionID, storagePath, err)
		}
		if size != expectedSize {
			return checked, fmt.Errorf("File Version storage object size mismatch: version_id=%s expected=%d actual=%d", versionID, expectedSize, size)
		}
		if expectedSHA.Valid && expectedSHA.String != "" && sha != expectedSHA.String {
			return checked, fmt.Errorf("File Version storage object checksum mismatch: version_id=%s", versionID)
		}
		registered[filepath.ToSlash(filepath.Clean(filepath.FromSlash(storagePath)))] = struct{}{}
		checked++
	}
	return checked, rows.Err()
}

func checkDuplicateActiveTargetPaths(ctx context.Context, db *sql.DB) error {
	var path string
	var lowerName string
	var count int
	err := db.QueryRowContext(ctx, `
SELECT path, lower(name), COUNT(*)
FROM files
WHERE deleted_at IS NULL
GROUP BY path, lower(name)
HAVING COUNT(*) > 1
LIMIT 1`).Scan(&path, &lowerName, &count)
	if err == nil {
		return fmt.Errorf("duplicate active target path: path=%q name=%q count=%d", path, lowerName, count)
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("check duplicate active target paths: %w", err)
	}
	return nil
}

func checkCurrentStorageFiles(ctx context.Context, db *sql.DB, storageRoot string) (map[string]struct{}, int, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, storage_path, COALESCE(size, 0) FROM files WHERE is_dir = 0 ORDER BY id`)
	if err != nil {
		return nil, 0, fmt.Errorf("list storage references: %w", err)
	}
	defer rows.Close()
	registered := make(map[string]struct{})
	checked := 0
	for rows.Next() {
		var fileID string
		var storagePath string
		var expectedSize int64
		if err := rows.Scan(&fileID, &storagePath, &expectedSize); err != nil {
			return nil, 0, fmt.Errorf("scan storage reference: %w", err)
		}
		objectPath, err := safeJoin(storageRoot, storagePath)
		if err != nil {
			return nil, 0, fmt.Errorf("file %s: %w", fileID, err)
		}
		info, err := os.Stat(objectPath)
		if err != nil {
			return nil, 0, fmt.Errorf("storage object missing: file_id=%s path=%q: %w", fileID, storagePath, err)
		}
		if !info.Mode().IsRegular() || info.Size() != expectedSize {
			return nil, 0, fmt.Errorf("storage object size mismatch: file_id=%s expected=%d actual=%d", fileID, expectedSize, info.Size())
		}
		registered[filepath.ToSlash(filepath.Clean(filepath.FromSlash(storagePath)))] = struct{}{}
		checked++
	}
	return registered, checked, rows.Err()
}

func checkUnregisteredCurrentObjects(root string, registered map[string]struct{}) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && entry.Name() == ".staging" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, exists := registered[relative]; !exists {
			return fmt.Errorf("unregistered storage object %q", relative)
		}
		return nil
	})
}

func checkCurrentThumbnails(ctx context.Context, db *sql.DB, thumbnailRoot string) (int, error) {
	rows, err := db.QueryContext(ctx, `
SELECT file_id, thumbnail_path
FROM file_metadata
WHERE thumbnail_path IS NOT NULL AND thumbnail_path != ''
ORDER BY file_id`)
	if err != nil {
		return 0, fmt.Errorf("list thumbnail references: %w", err)
	}
	defer rows.Close()
	checked := 0
	for rows.Next() {
		var fileID string
		var thumbnailPath string
		if err := rows.Scan(&fileID, &thumbnailPath); err != nil {
			return 0, fmt.Errorf("scan thumbnail reference: %w", err)
		}
		path, err := safeJoin(thumbnailRoot, thumbnailPath)
		if err != nil {
			return 0, fmt.Errorf("thumbnail for file %s: %w", fileID, err)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			return 0, fmt.Errorf("dangling thumbnail: file_id=%s path=%q", fileID, thumbnailPath)
		}
		checked++
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return checked, nil
}
