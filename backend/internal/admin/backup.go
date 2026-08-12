// Package admin implements MemoDrive's offline backup and recovery operations.
package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/maintenance"

	_ "github.com/mattn/go-sqlite3"
)

const BackupFormatVersion = 1
const sha256HexLength = sha256.Size * 2

type BackupFile struct {
	FileID      string `json:"file_id"`
	VersionID   string `json:"version_id,omitempty"`
	StoragePath string `json:"storage_path"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
}

type BackupThumbnail struct {
	FileID string `json:"file_id"`
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type BackupManifest struct {
	FormatVersion      int               `json:"format_version"`
	AppVersion         string            `json:"app_version"`
	CreatedAt          time.Time         `json:"created_at"`
	DBSchemaMigrations []string          `json:"db_schema_migrations"`
	Files              []BackupFile      `json:"files"`
	FileVersions       []BackupFile      `json:"file_versions"`
	Thumbnails         []BackupThumbnail `json:"thumbnails"`
	DatabaseSHA256     string            `json:"database_sha256"`
	FileCount          int               `json:"file_count"`
	TotalBytes         int64             `json:"total_bytes"`
}

type BackupSummary struct {
	Command     string `json:"command"`
	Success     bool   `json:"success"`
	BackupPath  string `json:"backup_path"`
	ArchivePath string `json:"archive_path,omitempty"`
	FileCount   int    `json:"file_count"`
	TotalBytes  int64  `json:"total_bytes"`
}

type Manager struct {
	cfg        *config.Config
	appVersion string
	now        func() time.Time
}

func New(cfg *config.Config, appVersion string) *Manager {
	if strings.TrimSpace(appVersion) == "" {
		appVersion = "dev"
	}
	return &Manager{cfg: cfg, appVersion: appVersion, now: time.Now}
}

func (m *Manager) Backup(ctx context.Context, outputPath string) (BackupSummary, error) {
	writerLock, err := maintenance.AcquireWriterLock(m.cfg.Storage.DBPath)
	if err != nil {
		return BackupSummary{}, err
	}
	defer writerLock.Close()

	outputPath, err = filepath.Abs(outputPath)
	if err != nil {
		return BackupSummary{}, fmt.Errorf("resolve backup output: %w", err)
	}
	if _, err := os.Stat(outputPath); err == nil {
		return BackupSummary{}, fmt.Errorf("backup output already exists: %s", outputPath)
	} else if !os.IsNotExist(err) {
		return BackupSummary{}, fmt.Errorf("inspect backup output: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return BackupSummary{}, fmt.Errorf("create backup parent: %w", err)
	}

	workingPath, err := os.MkdirTemp(filepath.Dir(outputPath), ".memodrive-backup-*")
	if err != nil {
		return BackupSummary{}, fmt.Errorf("create backup working directory: %w", err)
	}
	defer os.RemoveAll(workingPath)

	for _, dir := range []string{"db", "files", "thumbnails", "config"} {
		if err := os.MkdirAll(filepath.Join(workingPath, dir), 0o755); err != nil {
			return BackupSummary{}, fmt.Errorf("create backup layout: %w", err)
		}
	}

	databaseSnapshot := filepath.Join(workingPath, "db", "memodrive.db")
	if err := createSQLiteSnapshot(ctx, m.cfg.Storage.DBPath, databaseSnapshot); err != nil {
		return BackupSummary{}, err
	}
	databaseSHA, _, err := fileDigest(databaseSnapshot)
	if err != nil {
		return BackupSummary{}, fmt.Errorf("checksum database snapshot: %w", err)
	}

	files, migrations, err := backupFilesFromSnapshot(ctx, databaseSnapshot)
	if err != nil {
		return BackupSummary{}, err
	}
	if files == nil {
		files = []BackupFile{}
	}
	fileVersions, err := backupFileVersionsFromSnapshot(ctx, databaseSnapshot)
	if err != nil {
		return BackupSummary{}, err
	}
	if fileVersions == nil {
		fileVersions = []BackupFile{}
	}
	var totalBytes int64
	for index := range files {
		source, err := safeJoin(m.cfg.Storage.Root, files[index].StoragePath)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("file %s: %w", files[index].FileID, err)
		}
		destination, err := safeJoin(filepath.Join(workingPath, "files"), files[index].StoragePath)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("file %s backup path: %w", files[index].FileID, err)
		}
		if err := copyFile(source, destination); err != nil {
			return BackupSummary{}, fmt.Errorf("backup file %s: %w", files[index].FileID, err)
		}
		sha, size, err := fileDigest(destination)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("checksum file %s: %w", files[index].FileID, err)
		}
		if size != files[index].Size {
			return BackupSummary{}, fmt.Errorf("file %s size mismatch: database=%d storage=%d", files[index].FileID, files[index].Size, size)
		}
		files[index].SHA256 = sha
		totalBytes += size
	}
	for index := range fileVersions {
		source, err := safeJoin(m.cfg.Storage.Root, fileVersions[index].StoragePath)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("File Version %s: %w", fileVersions[index].VersionID, err)
		}
		destination, err := safeJoin(filepath.Join(workingPath, "files"), fileVersions[index].StoragePath)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("File Version %s backup path: %w", fileVersions[index].VersionID, err)
		}
		if err := copyFile(source, destination); err != nil {
			return BackupSummary{}, fmt.Errorf("backup File Version %s: %w", fileVersions[index].VersionID, err)
		}
		sha, size, err := fileDigest(destination)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("checksum File Version %s: %w", fileVersions[index].VersionID, err)
		}
		if size != fileVersions[index].Size {
			return BackupSummary{}, fmt.Errorf("File Version %s size mismatch: database=%d storage=%d", fileVersions[index].VersionID, fileVersions[index].Size, size)
		}
		fileVersions[index].SHA256 = sha
		totalBytes += size
	}
	thumbnails, err := backupThumbnailsFromSnapshot(ctx, databaseSnapshot)
	if err != nil {
		return BackupSummary{}, err
	}
	if thumbnails == nil {
		thumbnails = []BackupThumbnail{}
	}
	for index := range thumbnails {
		source, err := safeJoin(m.cfg.Storage.ThumbnailDir, thumbnails[index].Path)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("thumbnail for file %s: %w", thumbnails[index].FileID, err)
		}
		destination, err := safeJoin(filepath.Join(workingPath, "thumbnails"), thumbnails[index].Path)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("thumbnail backup path for file %s: %w", thumbnails[index].FileID, err)
		}
		if err := copyFile(source, destination); err != nil {
			return BackupSummary{}, fmt.Errorf("backup thumbnail for file %s: %w", thumbnails[index].FileID, err)
		}
		sha, size, err := fileDigest(destination)
		if err != nil {
			return BackupSummary{}, fmt.Errorf("checksum thumbnail for file %s: %w", thumbnails[index].FileID, err)
		}
		thumbnails[index].SHA256 = sha
		thumbnails[index].Size = size
	}

	manifest := BackupManifest{
		FormatVersion:      BackupFormatVersion,
		AppVersion:         m.appVersion,
		CreatedAt:          m.now().UTC(),
		DBSchemaMigrations: migrations,
		Files:              files,
		FileVersions:       fileVersions,
		Thumbnails:         thumbnails,
		DatabaseSHA256:     databaseSHA,
		FileCount:          len(files),
		TotalBytes:         totalBytes,
	}
	if err := writeJSON(filepath.Join(workingPath, "manifest.json"), manifest, 0o644); err != nil {
		return BackupSummary{}, fmt.Errorf("write manifest: %w", err)
	}
	if err := writeJSON(filepath.Join(workingPath, "config", "effective-config.redacted.json"), redactedConfig(m.cfg), 0o600); err != nil {
		return BackupSummary{}, fmt.Errorf("write redacted config: %w", err)
	}
	if err := os.Rename(workingPath, outputPath); err != nil {
		return BackupSummary{}, fmt.Errorf("publish backup: %w", err)
	}

	return BackupSummary{
		Command:    "backup",
		Success:    true,
		BackupPath: outputPath,
		FileCount:  len(files),
		TotalBytes: totalBytes,
	}, nil
}

func backupFileVersionsFromSnapshot(ctx context.Context, databasePath string) ([]BackupFile, error) {
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(databasePath)+"?mode=ro&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open database snapshot File Version catalog: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
SELECT id, file_id, storage_path, size
FROM file_versions
ORDER BY storage_path, id`)
	if err != nil {
		return nil, fmt.Errorf("list backup File Versions: %w", err)
	}
	defer rows.Close()
	var versions []BackupFile
	for rows.Next() {
		var version BackupFile
		if err := rows.Scan(&version.VersionID, &version.FileID, &version.StoragePath, &version.Size); err != nil {
			return nil, fmt.Errorf("scan backup File Version: %w", err)
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func backupThumbnailsFromSnapshot(ctx context.Context, databasePath string) ([]BackupThumbnail, error) {
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(databasePath)+"?mode=ro&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("open database snapshot thumbnails: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, `
SELECT file_id, thumbnail_path
FROM file_metadata
WHERE thumbnail_path IS NOT NULL AND thumbnail_path != ''
ORDER BY thumbnail_path, file_id`)
	if err != nil {
		return nil, fmt.Errorf("list backup thumbnails: %w", err)
	}
	defer rows.Close()
	var thumbnails []BackupThumbnail
	for rows.Next() {
		var thumbnail BackupThumbnail
		if err := rows.Scan(&thumbnail.FileID, &thumbnail.Path); err != nil {
			return nil, fmt.Errorf("scan backup thumbnail: %w", err)
		}
		thumbnails = append(thumbnails, thumbnail)
	}
	return thumbnails, rows.Err()
}

func createSQLiteSnapshot(ctx context.Context, sourcePath, destinationPath string) error {
	db, err := sql.Open("sqlite3", sourcePath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open database for backup: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	if err := sqliteIntegrityCheck(ctx, db); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return fmt.Errorf("checkpoint database WAL: %w", err)
	}
	quotedDestination := strings.ReplaceAll(destinationPath, "'", "''")
	if _, err := db.ExecContext(ctx, "VACUUM INTO '"+quotedDestination+"'"); err != nil {
		return fmt.Errorf("create database snapshot: %w", err)
	}

	snapshot, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(destinationPath)+"?mode=ro&_foreign_keys=on")
	if err != nil {
		return fmt.Errorf("open database snapshot: %w", err)
	}
	defer snapshot.Close()
	if err := sqliteIntegrityCheck(ctx, snapshot); err != nil {
		return fmt.Errorf("verify database snapshot: %w", err)
	}
	return nil
}

func sqliteIntegrityCheck(ctx context.Context, db *sql.DB) error {
	var quickCheck string
	if err := db.QueryRowContext(ctx, `PRAGMA quick_check`).Scan(&quickCheck); err != nil {
		return fmt.Errorf("database quick_check: %w", err)
	}
	if quickCheck != "ok" {
		return fmt.Errorf("database quick_check failed: %s", quickCheck)
	}
	rows, err := db.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("database foreign_key_check: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("database foreign_key_check failed")
	}
	return rows.Err()
}

func backupFilesFromSnapshot(ctx context.Context, databasePath string) ([]BackupFile, []string, error) {
	db, err := sql.Open("sqlite3", "file:"+filepath.ToSlash(databasePath)+"?mode=ro&_foreign_keys=on")
	if err != nil {
		return nil, nil, fmt.Errorf("open database snapshot catalog: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `
SELECT id, storage_path, COALESCE(size, 0)
FROM files
WHERE is_dir = 0
ORDER BY storage_path, id`)
	if err != nil {
		return nil, nil, fmt.Errorf("list backup files: %w", err)
	}
	var files []BackupFile
	for rows.Next() {
		var file BackupFile
		if err := rows.Scan(&file.FileID, &file.StoragePath, &file.Size); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("scan backup file: %w", err)
		}
		files = append(files, file)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("close backup file catalog: %w", err)
	}

	migrationRows, err := db.QueryContext(ctx, `SELECT id FROM schema_migrations ORDER BY id`)
	if err != nil {
		return nil, nil, fmt.Errorf("list schema migrations: %w", err)
	}
	defer migrationRows.Close()
	var migrations []string
	for migrationRows.Next() {
		var migration string
		if err := migrationRows.Scan(&migration); err != nil {
			return nil, nil, fmt.Errorf("scan schema migration: %w", err)
		}
		migrations = append(migrations, migration)
	}
	return files, migrations, migrationRows.Err()
}

func copyFile(source, destination string) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source is not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func fileDigest(path string) (string, int64, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func safeJoin(root, relativePath string) (string, error) {
	cleaned := filepath.Clean(filepath.FromSlash(relativePath))
	if cleaned == "." || filepath.IsAbs(cleaned) || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe storage path %q", relativePath)
	}
	return filepath.Join(root, cleaned), nil
}

func writeJSON(path string, value any, mode os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, mode)
}

func redactedConfig(cfg *config.Config) map[string]any {
	values := map[string]any{
		"storage_root":                  cfg.Storage.Root,
		"db_path":                       cfg.Storage.DBPath,
		"thumbnail_dir":                 cfg.Storage.ThumbnailDir,
		"storage_quota_bytes":           cfg.Storage.QuotaBytes,
		"storage_reserved_bytes":        cfg.Storage.ReservedBytes,
		"storage_temp_limit_bytes":      cfg.Storage.TempLimitBytes,
		"webdav_enabled":                cfg.WebDAV.Enabled,
		"trash_retention_days":          cfg.Trash.RetentionDays,
		"janitor_sweep_storage_enabled": cfg.Janitor.SweepStorageEnabled,
		"openai_api_key":                "[REDACTED]",
		"transcribe_api_key":            "[REDACTED]",
		"admin_password":                "[REDACTED]",
		"jwt_secret":                    "[REDACTED]",
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(values))
	for _, key := range keys {
		ordered[key] = values[key]
	}
	return ordered
}
