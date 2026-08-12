package admin

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type VerifySummary struct {
	Command            string `json:"command"`
	Success            bool   `json:"success"`
	BackupPath         string `json:"backup_path"`
	FileCount          int    `json:"file_count"`
	VerifiedFiles      int    `json:"verified_files"`
	VerifiedThumbnails int    `json:"verified_thumbnails"`
	TotalBytes         int64  `json:"total_bytes"`
}

func (m *Manager) Verify(ctx context.Context, backupPath string, sample int) (VerifySummary, error) {
	backupPath, err := filepath.Abs(backupPath)
	if err != nil {
		return VerifySummary{}, fmt.Errorf("resolve backup path: %w", err)
	}
	manifest, err := readManifest(filepath.Join(backupPath, "manifest.json"))
	if err != nil {
		return VerifySummary{}, err
	}
	if manifest.FormatVersion != BackupFormatVersion {
		return VerifySummary{}, fmt.Errorf("unsupported backup format_version %d", manifest.FormatVersion)
	}
	if err := validateManifest(manifest); err != nil {
		return VerifySummary{}, err
	}
	registeredObjects := append(append([]BackupFile{}, manifest.Files...), manifest.FileVersions...)
	if err := verifyRegisteredObjects(filepath.Join(backupPath, "files"), registeredObjects); err != nil {
		return VerifySummary{}, err
	}

	databasePath := filepath.Join(backupPath, "db", "memodrive.db")
	databaseSHA, _, err := fileDigest(databasePath)
	if err != nil {
		return VerifySummary{}, fmt.Errorf("checksum backup database: %w", err)
	}
	if databaseSHA != manifest.DatabaseSHA256 {
		return VerifySummary{}, fmt.Errorf("database checksum mismatch")
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(databasePath)+"?mode=ro&_foreign_keys=on")
	if err != nil {
		return VerifySummary{}, fmt.Errorf("open backup database: %w", err)
	}
	defer db.Close()
	if err := sqliteIntegrityCheck(ctx, db); err != nil {
		return VerifySummary{}, err
	}
	if err := verifyMigrationManifest(ctx, db, manifest.DBSchemaMigrations); err != nil {
		return VerifySummary{}, err
	}
	if err := checkDuplicateActiveTargetPaths(ctx, db); err != nil {
		return VerifySummary{}, err
	}
	if err := verifyDatabaseFileCatalog(ctx, db, manifest.Files); err != nil {
		return VerifySummary{}, err
	}
	if err := verifyDatabaseFileVersionCatalog(ctx, db, manifest.FileVersions); err != nil {
		return VerifySummary{}, err
	}
	if err := verifyThumbnails(ctx, db, filepath.Join(backupPath, "thumbnails"), manifest.Thumbnails); err != nil {
		return VerifySummary{}, err
	}

	filesToVerify := registeredObjects
	if sample > 0 && sample < len(filesToVerify) {
		filesToVerify = filesToVerify[:sample]
	}
	for _, file := range filesToVerify {
		objectPath, err := safeJoin(filepath.Join(backupPath, "files"), file.StoragePath)
		if err != nil {
			return VerifySummary{}, fmt.Errorf("file %s: %w", file.FileID, err)
		}
		sha, size, err := fileDigest(objectPath)
		if err != nil {
			return VerifySummary{}, fmt.Errorf("verify file %s: %w", file.FileID, err)
		}
		if size != file.Size {
			return VerifySummary{}, fmt.Errorf("file %s size mismatch: manifest=%d actual=%d", file.FileID, file.Size, size)
		}
		if sha != file.SHA256 {
			return VerifySummary{}, fmt.Errorf("file %s checksum mismatch", file.FileID)
		}
	}

	return VerifySummary{
		Command:            "verify",
		Success:            true,
		BackupPath:         backupPath,
		FileCount:          manifest.FileCount,
		VerifiedFiles:      len(filesToVerify),
		VerifiedThumbnails: len(manifest.Thumbnails),
		TotalBytes:         manifest.TotalBytes,
	}, nil
}

func verifyDatabaseFileVersionCatalog(ctx context.Context, db *sql.DB, versions []BackupFile) error {
	manifestByID := make(map[string]BackupFile, len(versions))
	for _, version := range versions {
		if _, exists := manifestByID[version.VersionID]; exists {
			return fmt.Errorf("duplicate manifest File Version id %q", version.VersionID)
		}
		manifestByID[version.VersionID] = version
	}
	rows, err := db.QueryContext(ctx, `SELECT id, file_id, storage_path, size FROM file_versions ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read backup database File Version catalog: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]struct{}, len(versions))
	for rows.Next() {
		var versionID, fileID, storagePath string
		var size int64
		if err := rows.Scan(&versionID, &fileID, &storagePath, &size); err != nil {
			return fmt.Errorf("scan backup database File Version catalog: %w", err)
		}
		manifestVersion, exists := manifestByID[versionID]
		if !exists {
			return fmt.Errorf("database File Version missing from manifest: version_id=%s", versionID)
		}
		if manifestVersion.FileID != fileID || filepath.Clean(filepath.FromSlash(manifestVersion.StoragePath)) != filepath.Clean(filepath.FromSlash(storagePath)) || manifestVersion.Size != size {
			return fmt.Errorf("database File Version mismatch: version_id=%s", versionID)
		}
		seen[versionID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, version := range versions {
		if _, exists := seen[version.VersionID]; !exists {
			return fmt.Errorf("manifest File Version missing from database: version_id=%s", version.VersionID)
		}
	}
	return nil
}

func verifyMigrationManifest(ctx context.Context, db *sql.DB, expected []string) error {
	rows, err := db.QueryContext(ctx, `SELECT id FROM schema_migrations ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read backup database migrations: %w", err)
	}
	defer rows.Close()
	var actual []string
	for rows.Next() {
		var migration string
		if err := rows.Scan(&migration); err != nil {
			return fmt.Errorf("scan backup database migration: %w", err)
		}
		actual = append(actual, migration)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("migration manifest mismatch: manifest=%v database=%v", expected, actual)
	}
	for index := range actual {
		if actual[index] != expected[index] {
			return fmt.Errorf("migration manifest mismatch: manifest=%v database=%v", expected, actual)
		}
	}
	return nil
}

func verifyThumbnails(ctx context.Context, db *sql.DB, root string, thumbnails []BackupThumbnail) error {
	byFileID := make(map[string]BackupThumbnail, len(thumbnails))
	byPath := make(map[string]string, len(thumbnails))
	for _, thumbnail := range thumbnails {
		if _, exists := byFileID[thumbnail.FileID]; exists {
			return fmt.Errorf("duplicate thumbnail file_id %q", thumbnail.FileID)
		}
		cleanedPath := filepath.ToSlash(filepath.Clean(filepath.FromSlash(thumbnail.Path)))
		if previousID, exists := byPath[cleanedPath]; exists {
			return fmt.Errorf("duplicate thumbnail path %q for files %s and %s", cleanedPath, previousID, thumbnail.FileID)
		}
		byFileID[thumbnail.FileID] = thumbnail
		byPath[cleanedPath] = thumbnail.FileID
		path, err := safeJoin(root, thumbnail.Path)
		if err != nil {
			return fmt.Errorf("thumbnail for file %s: %w", thumbnail.FileID, err)
		}
		sha, size, err := fileDigest(path)
		if err != nil {
			return fmt.Errorf("verify thumbnail for file %s: %w", thumbnail.FileID, err)
		}
		if size != thumbnail.Size {
			return fmt.Errorf("thumbnail size mismatch: file_id=%s manifest=%d actual=%d", thumbnail.FileID, thumbnail.Size, size)
		}
		if sha != thumbnail.SHA256 {
			return fmt.Errorf("thumbnail checksum mismatch: file_id=%s", thumbnail.FileID)
		}
	}

	rows, err := db.QueryContext(ctx, `
SELECT file_id, thumbnail_path
FROM file_metadata
WHERE thumbnail_path IS NOT NULL AND thumbnail_path != ''
ORDER BY file_id`)
	if err != nil {
		return fmt.Errorf("read backup thumbnail catalog: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]struct{}, len(thumbnails))
	for rows.Next() {
		var fileID string
		var thumbnailPath string
		if err := rows.Scan(&fileID, &thumbnailPath); err != nil {
			return fmt.Errorf("scan backup thumbnail catalog: %w", err)
		}
		thumbnail, exists := byFileID[fileID]
		if !exists {
			return fmt.Errorf("database thumbnail missing from manifest: file_id=%s", fileID)
		}
		if filepath.Clean(filepath.FromSlash(thumbnail.Path)) != filepath.Clean(filepath.FromSlash(thumbnailPath)) {
			return fmt.Errorf("database thumbnail path mismatch: file_id=%s", fileID)
		}
		seen[fileID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, thumbnail := range thumbnails {
		if _, exists := seen[thumbnail.FileID]; !exists {
			return fmt.Errorf("manifest thumbnail missing from database: file_id=%s", thumbnail.FileID)
		}
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported thumbnail object %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if _, exists := byPath[relative]; !exists {
			return fmt.Errorf("unregistered thumbnail %q", relative)
		}
		return nil
	})
}

func verifyDatabaseFileCatalog(ctx context.Context, db *sql.DB, files []BackupFile) error {
	manifestByID := make(map[string]BackupFile, len(files))
	for _, file := range files {
		if _, exists := manifestByID[file.FileID]; exists {
			return fmt.Errorf("duplicate manifest file_id %q", file.FileID)
		}
		manifestByID[file.FileID] = file
	}
	rows, err := db.QueryContext(ctx, `
SELECT id, storage_path, COALESCE(size, 0)
FROM files
WHERE is_dir = 0
ORDER BY id`)
	if err != nil {
		return fmt.Errorf("read backup database file catalog: %w", err)
	}
	defer rows.Close()
	seen := make(map[string]struct{}, len(files))
	for rows.Next() {
		var fileID string
		var storagePath string
		var size int64
		if err := rows.Scan(&fileID, &storagePath, &size); err != nil {
			return fmt.Errorf("scan backup database file catalog: %w", err)
		}
		manifestFile, exists := manifestByID[fileID]
		if !exists {
			return fmt.Errorf("database file missing from manifest: file_id=%s", fileID)
		}
		if filepath.Clean(filepath.FromSlash(manifestFile.StoragePath)) != filepath.Clean(filepath.FromSlash(storagePath)) {
			return fmt.Errorf("database file storage path mismatch: file_id=%s", fileID)
		}
		if manifestFile.Size != size {
			return fmt.Errorf("database file size mismatch: file_id=%s", fileID)
		}
		seen[fileID] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read backup database file catalog: %w", err)
	}
	for _, file := range files {
		if _, exists := seen[file.FileID]; !exists {
			return fmt.Errorf("manifest file missing from database: file_id=%s", file.FileID)
		}
	}
	return nil
}

func verifyRegisteredObjects(root string, files []BackupFile) error {
	registered := make(map[string]string, len(files))
	for _, file := range files {
		cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.StoragePath)))
		if previousID, exists := registered[cleaned]; exists {
			return fmt.Errorf("duplicate storage path %q for files %s and %s", cleaned, previousID, file.FileID)
		}
		registered[cleaned] = file.FileID
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("scan backup files: %w", walkErr)
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported storage object %q", path)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("resolve backup object path: %w", err)
		}
		relative = filepath.ToSlash(relative)
		if _, exists := registered[relative]; !exists {
			return fmt.Errorf("unregistered storage object %q", relative)
		}
		return nil
	})
}

func readManifest(path string) (BackupManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return BackupManifest{}, fmt.Errorf("open manifest: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest BackupManifest
	if err := decoder.Decode(&manifest); err != nil {
		return BackupManifest{}, fmt.Errorf("decode manifest: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return BackupManifest{}, fmt.Errorf("decode manifest: trailing JSON content")
	}
	return manifest, nil
}

func validateManifest(manifest BackupManifest) error {
	if manifest.AppVersion == "" || manifest.CreatedAt.IsZero() || len(manifest.DatabaseSHA256) != sha256HexLength {
		return fmt.Errorf("manifest schema validation failed: missing required metadata")
	}
	if manifest.FileCount != len(manifest.Files) {
		return fmt.Errorf("manifest file_count mismatch: declared=%d actual=%d", manifest.FileCount, len(manifest.Files))
	}
	var totalBytes int64
	for _, file := range manifest.Files {
		if file.FileID == "" || file.StoragePath == "" || file.Size < 0 || len(file.SHA256) != sha256HexLength {
			return fmt.Errorf("manifest schema validation failed: invalid file entry")
		}
		totalBytes += file.Size
	}
	for _, version := range manifest.FileVersions {
		if version.VersionID == "" || version.FileID == "" || version.StoragePath == "" || version.Size < 0 || len(version.SHA256) != sha256HexLength {
			return fmt.Errorf("manifest schema validation failed: invalid File Version entry")
		}
		totalBytes += version.Size
	}
	if totalBytes != manifest.TotalBytes {
		return fmt.Errorf("manifest total_bytes mismatch: declared=%d actual=%d", manifest.TotalBytes, totalBytes)
	}
	for _, thumbnail := range manifest.Thumbnails {
		if thumbnail.FileID == "" || thumbnail.Path == "" || thumbnail.Size < 0 || len(thumbnail.SHA256) != sha256HexLength {
			return fmt.Errorf("manifest schema validation failed: invalid thumbnail entry")
		}
	}
	return nil
}
