package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/admin"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/handler"
	"github.com/memodrive/backend/internal/maintenance"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func TestBackupCommandCreatesSelfDescribingDirectoryBackup(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	storageRoot := filepath.Join(workspace, "data", "files")
	databasePath := filepath.Join(workspace, "data", "db", "memodrive.db")
	thumbnailDir := filepath.Join(workspace, "data", "thumbnails")
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         storageRoot,
		DBPath:       databasePath,
		TempDir:      filepath.Join(workspace, "data", "tmp"),
		ThumbnailDir: thumbnailDir,
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}

	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	file := &model.File{
		ID:          "file-1",
		Name:        "你好.md",
		Path:        "/Notes",
		StoragePath: filepath.Join("file-1", "你好.md"),
		Size:        int64(len("backup me")),
		MimeType:    "text/markdown",
		Status:      "uploaded",
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create fixture file: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(storageRoot, file.StoragePath)), 0o755); err != nil {
		t.Fatalf("create fixture object directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, file.StoragePath), []byte("backup me"), 0o644); err != nil {
		t.Fatalf("write fixture object: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	envPath := filepath.Join(workspace, "memodrive.env")
	envText := fmt.Sprintf("STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		storageRoot,
		databasePath,
		cfg.Storage.TempDir,
		thumbnailDir,
	)
	if err := os.WriteFile(envPath, []byte(envText), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	backupDir := filepath.Join(workspace, "backups", "backup-1")
	var stdout bytes.Buffer
	exitCode := run(context.Background(), []string{
		"backup",
		"--output", backupDir,
		"--env-file", envPath,
	}, &stdout)
	if exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, stdout.String())
	}

	var summary struct {
		Command   string `json:"command"`
		Success   bool   `json:"success"`
		FileCount int    `json:"file_count"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("decode command output: %v; output = %s", err, stdout.String())
	}
	if !summary.Success || summary.Command != "backup" || summary.FileCount != 1 {
		t.Fatalf("unexpected command summary: %+v", summary)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest struct {
		FormatVersion  int    `json:"format_version"`
		FileCount      int    `json:"file_count"`
		DatabaseSHA256 string `json:"database_sha256"`
		Files          []struct {
			FileID      string `json:"file_id"`
			StoragePath string `json:"storage_path"`
			SHA256      string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.FormatVersion != 1 || manifest.FileCount != 1 || manifest.DatabaseSHA256 == "" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if len(manifest.Files) != 1 || manifest.Files[0].FileID != file.ID || manifest.Files[0].SHA256 == "" {
		t.Fatalf("unexpected manifest files: %+v", manifest.Files)
	}

	backedUpObject, err := os.ReadFile(filepath.Join(backupDir, "files", file.StoragePath))
	if err != nil {
		t.Fatalf("read backed-up object: %v", err)
	}
	if string(backedUpObject) != "backup me" {
		t.Fatalf("backed-up object = %q", backedUpObject)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "db", "memodrive.db")); err != nil {
		t.Fatalf("database snapshot missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "config", "effective-config.redacted.json")); err != nil {
		t.Fatalf("redacted config missing: %v", err)
	}
}

func TestVerifyCommandAcceptsBackupCreatedByMemoDrive(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	storageRoot := filepath.Join(workspace, "data", "files")
	databasePath := filepath.Join(workspace, "data", "db", "memodrive.db")
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         storageRoot,
		DBPath:       databasePath,
		TempDir:      filepath.Join(workspace, "data", "tmp"),
		ThumbnailDir: filepath.Join(workspace, "data", "thumbnails"),
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	file := &model.File{
		ID:          "verify-file",
		Name:        "verify.txt",
		Path:        "/",
		StoragePath: filepath.Join("verify-file", "verify.txt"),
		Size:        int64(len("verified")),
		MimeType:    "text/plain",
		Status:      "uploaded",
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create fixture file: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	objectPath := filepath.Join(storageRoot, file.StoragePath)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("create fixture object directory: %v", err)
	}
	if err := os.WriteFile(objectPath, []byte("verified"), 0o644); err != nil {
		t.Fatalf("write fixture object: %v", err)
	}
	envPath := filepath.Join(workspace, "memodrive.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf(
		"STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		storageRoot,
		databasePath,
		cfg.Storage.TempDir,
		cfg.Storage.ThumbnailDir,
	)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	backupDir := filepath.Join(workspace, "backup")
	var backupOutput bytes.Buffer
	if exitCode := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", envPath}, &backupOutput); exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, backupOutput.String())
	}

	var verifyOutput bytes.Buffer
	exitCode := run(context.Background(), []string{"verify", "--backup", backupDir}, &verifyOutput)
	if exitCode != 0 {
		t.Fatalf("verify exit code = %d, output = %s", exitCode, verifyOutput.String())
	}
	var summary struct {
		Command       string `json:"command"`
		Success       bool   `json:"success"`
		VerifiedFiles int    `json:"verified_files"`
	}
	if err := json.Unmarshal(verifyOutput.Bytes(), &summary); err != nil {
		t.Fatalf("decode verify output: %v; output = %s", err, verifyOutput.String())
	}
	if !summary.Success || summary.Command != "verify" || summary.VerifiedFiles != 1 {
		t.Fatalf("unexpected verify summary: %+v", summary)
	}
}

func TestBackupAndVerifyIncludeFileVersions(t *testing.T) {
	workspace := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(workspace, "data", "files"),
			DBPath:       filepath.Join(workspace, "data", "db", "memodrive.db"),
			TempDir:      filepath.Join(workspace, "data", "tmp"),
			ThumbnailDir: filepath.Join(workspace, "data", "thumbnails"),
		},
		FileVersion: config.FileVersionConfig{Enabled: true},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	file := &model.File{
		ID:          "versioned-backup-file",
		Name:        "history.md",
		Path:        "/",
		StoragePath: "history.md",
		Size:        3,
		MimeType:    "text/markdown",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create fixture File: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Storage.Root, file.StoragePath), []byte("old"), 0o644); err != nil {
		t.Fatalf("write fixture File: %v", err)
	}
	files := service.NewFileService(cfg, db, nil)
	base, err := files.MarkdownContent(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("read fixture Markdown: %v", err)
	}
	if _, err := files.UpdateMarkdownContent(context.Background(), file.ID, "new", base.UpdatedAt); err != nil {
		t.Fatalf("create fixture File Version: %v", err)
	}
	versions, err := files.ListVersions(context.Background(), file.ID)
	if err != nil || len(versions) != 1 {
		t.Fatalf("list fixture File Versions: versions=%#v err=%v", versions, err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}

	envPath := filepath.Join(workspace, "memodrive.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf(
		"STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		cfg.Storage.Root,
		cfg.Storage.DBPath,
		cfg.Storage.TempDir,
		cfg.Storage.ThumbnailDir,
	)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	backupDir := filepath.Join(workspace, "backup")
	var backupOutput bytes.Buffer
	if code := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", envPath}, &backupOutput); code != 0 {
		t.Fatalf("backup exit code = %d: %s", code, backupOutput.String())
	}
	manifestBytes, err := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read backup manifest: %v", err)
	}
	var manifest admin.BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode backup manifest: %v", err)
	}
	if len(manifest.FileVersions) != 1 || manifest.FileVersions[0].FileID != file.ID ||
		manifest.FileVersions[0].StoragePath != versions[0].StoragePath || manifest.FileVersions[0].SHA256 == "" {
		t.Fatalf("unexpected manifest File Versions %#v", manifest.FileVersions)
	}
	backedUpVersion, err := os.ReadFile(filepath.Join(backupDir, "files", filepath.FromSlash(versions[0].StoragePath)))
	if err != nil {
		t.Fatalf("read backed-up File Version: %v", err)
	}
	if string(backedUpVersion) != "old" {
		t.Fatalf("backed-up File Version content = %q, want old", backedUpVersion)
	}
	var verifyOutput bytes.Buffer
	if code := run(context.Background(), []string{"verify", "--backup", backupDir}, &verifyOutput); code != 0 {
		t.Fatalf("verify exit code = %d: %s", code, verifyOutput.String())
	}
	targetRoot := filepath.Join(workspace, "restored", "files")
	targetDB := filepath.Join(workspace, "restored", "db", "memodrive.db")
	var restoreOutput bytes.Buffer
	if code := run(context.Background(), []string{
		"restore", "--backup", backupDir, "--target-root", targetRoot, "--target-db", targetDB,
	}, &restoreOutput); code != 0 {
		t.Fatalf("restore exit code = %d: %s", code, restoreOutput.String())
	}
	restoredCfg := &config.Config{Storage: config.StorageConfig{Root: targetRoot, DBPath: targetDB}}
	restoredDB, err := store.Open(context.Background(), restoredCfg)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restoredDB.Close()
	restoredFiles := service.NewFileService(restoredCfg, restoredDB, nil)
	restoredVersions, err := restoredFiles.ListVersions(context.Background(), file.ID)
	if err != nil || len(restoredVersions) != 1 {
		t.Fatalf("list restored File Versions: versions=%#v err=%v", restoredVersions, err)
	}
	_, restoredVersionPath, err := restoredFiles.VersionDownload(context.Background(), file.ID, restoredVersions[0].ID)
	if err != nil {
		t.Fatalf("open restored File Version: %v", err)
	}
	restoredVersionContent, err := os.ReadFile(restoredVersionPath)
	if err != nil {
		t.Fatalf("read restored File Version: %v", err)
	}
	if string(restoredVersionContent) != "old" {
		t.Fatalf("restored File Version content = %q, want old", restoredVersionContent)
	}
}

func TestVerifyCommandRejectsUnregisteredStorageObject(t *testing.T) {
	t.Parallel()

	backupDir := createBackupFixture(t, "registered.txt", "registered")
	orphanPath := filepath.Join(backupDir, "files", "orphan", "unknown.txt")
	if err := os.MkdirAll(filepath.Dir(orphanPath), 0o755); err != nil {
		t.Fatalf("create orphan directory: %v", err)
	}
	if err := os.WriteFile(orphanPath, []byte("not in manifest"), 0o644); err != nil {
		t.Fatalf("write orphan object: %v", err)
	}

	var output bytes.Buffer
	exitCode := run(context.Background(), []string{"verify", "--backup", backupDir}, &output)
	if exitCode != 1 {
		t.Fatalf("verify exit code = %d, want 1; output = %s", exitCode, output.String())
	}
	if !strings.Contains(output.String(), "unregistered storage object") {
		t.Fatalf("verify output does not identify orphan: %s", output.String())
	}
}

func TestVerifyCommandRejectsDatabaseFileMissingFromManifest(t *testing.T) {
	t.Parallel()

	backupDir := createBackupFixture(t, "missing.txt", "missing")
	manifestPath := filepath.Join(backupDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest admin.BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest.Files = []admin.BackupFile{}
	manifest.FileCount = 0
	manifest.TotalBytes = 0
	manifestBytes, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, append(manifestBytes, '\n'), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}
	if err := os.Remove(filepath.Join(backupDir, "files", "fixture-file", "missing.txt")); err != nil {
		t.Fatalf("remove registered object: %v", err)
	}

	var output bytes.Buffer
	exitCode := run(context.Background(), []string{"verify", "--backup", backupDir}, &output)
	if exitCode != 1 {
		t.Fatalf("verify exit code = %d, want 1; output = %s", exitCode, output.String())
	}
	if !strings.Contains(output.String(), "database file missing from manifest") {
		t.Fatalf("verify output does not identify missing manifest entry: %s", output.String())
	}
}

func TestBackupCommandRejectsActiveWriter(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         filepath.Join(workspace, "data", "files"),
		DBPath:       filepath.Join(workspace, "data", "db", "memodrive.db"),
		TempDir:      filepath.Join(workspace, "data", "tmp"),
		ThumbnailDir: filepath.Join(workspace, "data", "thumbnails"),
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	writerLock, err := maintenance.AcquireWriterLock(cfg.Storage.DBPath)
	if err != nil {
		t.Fatalf("acquire writer lock: %v", err)
	}
	defer writerLock.Close()

	envPath := filepath.Join(workspace, "memodrive.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf(
		"STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		cfg.Storage.Root,
		cfg.Storage.DBPath,
		cfg.Storage.TempDir,
		cfg.Storage.ThumbnailDir,
	)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}

	var output bytes.Buffer
	exitCode := run(context.Background(), []string{
		"backup",
		"--output", filepath.Join(workspace, "backup"),
		"--env-file", envPath,
	}, &output)
	if exitCode != 1 {
		t.Fatalf("backup exit code = %d, want 1; output = %s", exitCode, output.String())
	}
	if !strings.Contains(output.String(), "active writer") {
		t.Fatalf("backup output does not identify active writer: %s", output.String())
	}
}

func TestBackupCommandIncludesReferencedThumbnail(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         filepath.Join(workspace, "data", "files"),
		DBPath:       filepath.Join(workspace, "data", "db", "memodrive.db"),
		TempDir:      filepath.Join(workspace, "data", "tmp"),
		ThumbnailDir: filepath.Join(workspace, "data", "thumbnails"),
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	file := &model.File{ID: "photo-1", Name: "photo.jpg", Path: "/", StoragePath: "photo-1/photo.jpg", Size: 5, MimeType: "image/jpeg", Status: "ready"}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create fixture file: %v", err)
	}
	thumbnailName := "photo-1.jpg"
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{FileID: file.ID, MetaJSON: `{}`, ThumbnailPath: &thumbnailName}); err != nil {
		t.Fatalf("create fixture metadata: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(cfg.Storage.Root, file.StoragePath)), 0o755); err != nil {
		t.Fatalf("create object directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Storage.Root, file.StoragePath), []byte("photo"), 0o644); err != nil {
		t.Fatalf("write object: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Storage.ThumbnailDir, thumbnailName), []byte("thumbnail"), 0o644); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Storage.ThumbnailDir, "orphan.jpg"), []byte("orphan"), 0o644); err != nil {
		t.Fatalf("write orphan thumbnail: %v", err)
	}
	envPath := filepath.Join(workspace, "memodrive.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf(
		"STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		cfg.Storage.Root, cfg.Storage.DBPath, cfg.Storage.TempDir, cfg.Storage.ThumbnailDir,
	)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	backupDir := filepath.Join(workspace, "backup")
	var output bytes.Buffer
	if exitCode := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", envPath}, &output); exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, output.String())
	}

	manifestBytes, err := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest admin.BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if len(manifest.Thumbnails) != 1 || manifest.Thumbnails[0].FileID != file.ID || manifest.Thumbnails[0].Path != thumbnailName {
		t.Fatalf("unexpected thumbnail manifest: %+v", manifest.Thumbnails)
	}
	if got, err := os.ReadFile(filepath.Join(backupDir, "thumbnails", thumbnailName)); err != nil || string(got) != "thumbnail" {
		t.Fatalf("backed-up thumbnail = %q, err = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(backupDir, "thumbnails", "orphan.jpg")); !os.IsNotExist(err) {
		t.Fatalf("orphan thumbnail should be excluded, err = %v", err)
	}
}

func TestIntegrityCommandChecksConfiguredDatabaseAndStorage(t *testing.T) {
	t.Parallel()

	fixture := createSourceFixture(t, "integrity.txt", "integrity")
	var output bytes.Buffer
	exitCode := run(context.Background(), []string{"integrity", "--env-file", fixture.EnvPath}, &output)
	if exitCode != 0 {
		t.Fatalf("integrity exit code = %d, output = %s", exitCode, output.String())
	}
	var summary struct {
		Command      string `json:"command"`
		Success      bool   `json:"success"`
		CheckedFiles int    `json:"checked_files"`
	}
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil {
		t.Fatalf("decode integrity output: %v; output = %s", err, output.String())
	}
	if !summary.Success || summary.Command != "integrity" || summary.CheckedFiles != 1 {
		t.Fatalf("unexpected integrity summary: %+v", summary)
	}
}

func TestRestoreCommandRestoresBackupToNewTarget(t *testing.T) {
	t.Parallel()

	backupDir := createBackupFixture(t, "恢复.txt", "restore me")
	targetBase := t.TempDir()
	targetRoot := filepath.Join(targetBase, "data", "files")
	targetDB := filepath.Join(targetBase, "data", "db", "memodrive.db")
	var output bytes.Buffer
	exitCode := run(context.Background(), []string{
		"restore",
		"--backup", backupDir,
		"--target-root", targetRoot,
		"--target-db", targetDB,
	}, &output)
	if exitCode != 0 {
		t.Fatalf("restore exit code = %d, output = %s", exitCode, output.String())
	}
	var summary struct {
		Command       string `json:"command"`
		Success       bool   `json:"success"`
		RestoredFiles int    `json:"restored_files"`
	}
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil {
		t.Fatalf("decode restore output: %v; output = %s", err, output.String())
	}
	if !summary.Success || summary.Command != "restore" || summary.RestoredFiles != 1 {
		t.Fatalf("unexpected restore summary: %+v", summary)
	}

	restoredConfig := &config.Config{Storage: config.StorageConfig{DBPath: targetDB}}
	restoredStore, err := store.Open(context.Background(), restoredConfig)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restoredStore.Close()
	restoredFile, err := restoredStore.GetFile(context.Background(), "fixture-file")
	if err != nil {
		t.Fatalf("read restored file record: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(targetRoot, restoredFile.StoragePath))
	if err != nil {
		t.Fatalf("read restored object: %v", err)
	}
	if string(content) != "restore me" {
		t.Fatalf("restored object = %q", content)
	}
}

func TestVerifyCommandRejectsTamperedThumbnail(t *testing.T) {
	t.Parallel()

	backupDir, thumbnailName := createThumbnailBackupFixture(t)
	if err := os.WriteFile(filepath.Join(backupDir, "thumbnails", thumbnailName), []byte("tampered!"), 0o644); err != nil {
		t.Fatalf("tamper thumbnail: %v", err)
	}
	var output bytes.Buffer
	exitCode := run(context.Background(), []string{"verify", "--backup", backupDir}, &output)
	if exitCode != 1 {
		t.Fatalf("verify exit code = %d, want 1; output = %s", exitCode, output.String())
	}
	if !strings.Contains(output.String(), "thumbnail") || !strings.Contains(output.String(), "checksum mismatch") {
		t.Fatalf("verify output does not identify thumbnail checksum: %s", output.String())
	}
}

func TestRestoreForceRemovesOldSQLiteSidecars(t *testing.T) {
	t.Parallel()

	backupDir := createBackupFixture(t, "new.txt", "new data")
	targetBase := t.TempDir()
	targetRoot := filepath.Join(targetBase, "data", "files")
	targetDB := filepath.Join(targetBase, "data", "db", "memodrive.db")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatalf("create old target root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, "old.txt"), []byte("old data"), 0o644); err != nil {
		t.Fatalf("write old target object: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(targetDB), 0o755); err != nil {
		t.Fatalf("create old database directory: %v", err)
	}
	for _, path := range []string{targetDB, targetDB + "-wal", targetDB + "-shm"} {
		if err := os.WriteFile(path, []byte("old sqlite state"), 0o600); err != nil {
			t.Fatalf("write old database state %s: %v", path, err)
		}
	}

	var output bytes.Buffer
	exitCode := run(context.Background(), []string{
		"restore",
		"--backup", backupDir,
		"--target-root", targetRoot,
		"--target-db", targetDB,
		"--force",
	}, &output)
	if exitCode != 0 {
		t.Fatalf("restore exit code = %d, output = %s", exitCode, output.String())
	}
	for _, sidecar := range []string{targetDB + "-wal", targetDB + "-shm"} {
		if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
			t.Fatalf("old SQLite sidecar remains after restore: %s, err = %v", sidecar, err)
		}
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "old.txt")); !os.IsNotExist(err) {
		t.Fatalf("old storage object remains after forced restore, err = %v", err)
	}
}

func TestReindexAllRebuildsFileThroughPipeline(t *testing.T) {
	t.Parallel()

	fixture := createSourceFixture(t, "reindex.md", "# Reindex\n\nhello from backup")
	var upsertCalls int
	services := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/embed":
			var body struct {
				Input []string `json:"input"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				http.Error(response, err.Error(), http.StatusBadRequest)
				return
			}
			embeddings := make([][]float32, len(body.Input))
			for index := range embeddings {
				embeddings[index] = []float32{0.1, 0.2, 0.3}
			}
			_ = json.NewEncoder(response).Encode(map[string]any{"embeddings": embeddings})
		case request.Method == http.MethodGet && strings.Contains(request.URL.Path, "/collections/"):
			_ = json.NewEncoder(response).Encode(map[string]any{"id": "collection-1", "name": "memodrive"})
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/upsert"):
			upsertCalls++
			response.WriteHeader(http.StatusOK)
		default:
			http.Error(response, "unexpected request: "+request.Method+" "+request.URL.Path, http.StatusNotFound)
		}
	}))
	defer services.Close()
	env, err := os.OpenFile(fixture.EnvPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open fixture env: %v", err)
	}
	if _, err := fmt.Fprintf(env, "OLLAMA_BASE_URL=%s\nCHROMA_BASE_URL=%s\nPIPELINE_WORKERS=1\n", services.URL, services.URL); err != nil {
		t.Fatalf("extend fixture env: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close fixture env: %v", err)
	}

	var output bytes.Buffer
	exitCode := run(context.Background(), []string{"reindex", "--all", "--env-file", fixture.EnvPath}, &output)
	if exitCode != 0 {
		t.Fatalf("reindex exit code = %d, output = %s", exitCode, output.String())
	}
	var summary struct {
		Command        string `json:"command"`
		Success        bool   `json:"success"`
		ReindexedFiles int    `json:"reindexed_files"`
		FailedFiles    int    `json:"failed_files"`
	}
	if err := json.Unmarshal(output.Bytes(), &summary); err != nil {
		t.Fatalf("decode reindex output: %v; output = %s", err, output.String())
	}
	if !summary.Success || summary.Command != "reindex" || summary.ReindexedFiles != 1 || summary.FailedFiles != 0 {
		t.Fatalf("unexpected reindex summary: %+v", summary)
	}
	if upsertCalls == 0 {
		t.Fatal("reindex did not upsert rebuilt vectors")
	}
	db, err := store.Open(context.Background(), fixture.Config)
	if err != nil {
		t.Fatalf("open reindexed database: %v", err)
	}
	defer db.Close()
	reindexed, err := db.GetFile(context.Background(), fixture.File.ID)
	if err != nil {
		t.Fatalf("read reindexed file: %v", err)
	}
	if reindexed.Status != "ready" || reindexed.ChunkCount == 0 {
		t.Fatalf("reindexed file = %+v", reindexed)
	}
}

func TestBackupEmptyDatabaseUsesStableManifestArrays(t *testing.T) {
	t.Parallel()

	workspace := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         filepath.Join(workspace, "data", "files"),
		DBPath:       filepath.Join(workspace, "data", "db", "memodrive.db"),
		TempDir:      filepath.Join(workspace, "data", "tmp"),
		ThumbnailDir: filepath.Join(workspace, "data", "thumbnails"),
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	envPath := filepath.Join(workspace, "memodrive.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf(
		"STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		cfg.Storage.Root, cfg.Storage.DBPath, cfg.Storage.TempDir, cfg.Storage.ThumbnailDir,
	)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	backupDir := filepath.Join(workspace, "backup")
	var output bytes.Buffer
	if exitCode := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", envPath}, &output); exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, output.String())
	}
	manifestBytes, err := os.ReadFile(filepath.Join(backupDir, "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest admin.BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	if manifest.Files == nil || manifest.FileVersions == nil || manifest.Thumbnails == nil {
		t.Fatalf("manifest collections must be arrays, got files=%v versions=%v thumbnails=%v", manifest.Files, manifest.FileVersions, manifest.Thumbnails)
	}
}

func TestBackupCommandOptionallyCreatesZIP64Archive(t *testing.T) {
	t.Parallel()

	fixture := createSourceFixture(t, "archive.txt", "archive")
	backupDir := filepath.Join(fixture.Workspace, "backup")
	archivePath := filepath.Join(fixture.Workspace, "archives", "backup.zip")
	var output bytes.Buffer
	exitCode := run(context.Background(), []string{
		"backup",
		"--output", backupDir,
		"--archive", archivePath,
		"--env-file", fixture.EnvPath,
	}, &output)
	if exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, output.String())
	}
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("open backup archive: %v", err)
	}
	defer archive.Close()
	foundManifest := false
	for _, file := range archive.File {
		if file.Name == "manifest.json" {
			foundManifest = true
			break
		}
	}
	if !foundManifest {
		t.Fatal("backup archive does not contain manifest.json")
	}
}

func TestBackupRestoreDrillPreservesActiveTrashUnicodeAndContentHashes(t *testing.T) {
	t.Parallel()

	activeContent := bytes.Repeat([]byte("大文件内容-"), 128)
	fixture := createSourceFixture(t, "报告-你好.md", string(activeContent))
	db, err := store.Open(context.Background(), fixture.Config)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	trashContent := []byte("trashed content")
	folder := &model.File{ID: "folder-1", Name: "资料", Path: "/", IsDir: true, MimeType: "inode/directory", Status: "ready"}
	if err := db.CreateFile(context.Background(), folder); err != nil {
		t.Fatalf("create folder fixture: %v", err)
	}
	trashFile := &model.File{
		ID:          "trash-file",
		Name:        "旧文件.txt",
		Path:        "/Archive",
		StoragePath: filepath.Join("trash-file", "旧文件.txt"),
		Size:        int64(len(trashContent)),
		MimeType:    "text/plain",
		Status:      "ready",
	}
	if err := db.CreateFile(context.Background(), trashFile); err != nil {
		t.Fatalf("create trash fixture: %v", err)
	}
	if err := db.SoftDeleteFile(context.Background(), trashFile.ID, trashFile.ID); err != nil {
		t.Fatalf("trash fixture file: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	trashObject := filepath.Join(fixture.Config.Storage.Root, trashFile.StoragePath)
	if err := os.MkdirAll(filepath.Dir(trashObject), 0o755); err != nil {
		t.Fatalf("create trash object directory: %v", err)
	}
	if err := os.WriteFile(trashObject, trashContent, 0o644); err != nil {
		t.Fatalf("write trash object: %v", err)
	}
	env, err := os.OpenFile(fixture.EnvPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("open fixture env: %v", err)
	}
	if _, err := fmt.Fprintln(env, "UPLOAD_CHUNK_SIZE=1024"); err != nil {
		t.Fatalf("configure fixture chunk size: %v", err)
	}
	if err := env.Close(); err != nil {
		t.Fatalf("close fixture env: %v", err)
	}

	backupDir := filepath.Join(fixture.Workspace, "drill-backup")
	var backupOutput bytes.Buffer
	if exitCode := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", fixture.EnvPath}, &backupOutput); exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, backupOutput.String())
	}
	var verifyOutput bytes.Buffer
	if exitCode := run(context.Background(), []string{"verify", "--backup", backupDir}, &verifyOutput); exitCode != 0 {
		t.Fatalf("verify exit code = %d, output = %s", exitCode, verifyOutput.String())
	}
	if err := os.RemoveAll(filepath.Join(fixture.Workspace, "data")); err != nil {
		t.Fatalf("remove source runtime copy: %v", err)
	}

	targetBase := t.TempDir()
	targetRoot := filepath.Join(targetBase, "data", "files")
	targetDB := filepath.Join(targetBase, "data", "db", "memodrive.db")
	var restoreOutput bytes.Buffer
	if exitCode := run(context.Background(), []string{
		"restore", "--backup", backupDir, "--target-root", targetRoot, "--target-db", targetDB,
	}, &restoreOutput); exitCode != 0 {
		t.Fatalf("restore exit code = %d, output = %s", exitCode, restoreOutput.String())
	}
	restoredConfig := &config.Config{Storage: config.StorageConfig{
		Root:         targetRoot,
		DBPath:       targetDB,
		ThumbnailDir: filepath.Join(targetBase, "data", "thumbnails"),
	}}
	restoredStore, err := store.Open(context.Background(), restoredConfig)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restoredStore.Close()
	restoredActive, err := restoredStore.GetFile(context.Background(), fixture.File.ID)
	if err != nil {
		t.Fatalf("read restored active file: %v", err)
	}
	restoredTrash, err := restoredStore.GetFileIncludeDeleted(context.Background(), trashFile.ID)
	if err != nil {
		t.Fatalf("read restored trash file: %v", err)
	}
	if restoredTrash.DeletedAt == nil {
		t.Fatal("restored Trash Entry lost deleted_at")
	}
	if _, err := restoredStore.GetFile(context.Background(), folder.ID); err != nil {
		t.Fatalf("read restored folder: %v", err)
	}
	for _, item := range []struct {
		file    *model.File
		content []byte
	}{
		{file: restoredActive, content: activeContent},
		{file: restoredTrash, content: trashContent},
	} {
		restoredContent, err := os.ReadFile(filepath.Join(targetRoot, item.file.StoragePath))
		if err != nil {
			t.Fatalf("read restored object %s: %v", item.file.ID, err)
		}
		if sha256.Sum256(restoredContent) != sha256.Sum256(item.content) {
			t.Fatalf("restored object hash mismatch: file_id=%s", item.file.ID)
		}
	}

	app := fiber.New()
	handler.NewFileHandler(service.NewFileService(restoredConfig, restoredStore, nil), nil).Register(app.Group("/api"))
	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/files/"+fixture.File.ID+"/download", nil))
	if err != nil {
		t.Fatalf("download restored file through API: %v", err)
	}
	defer response.Body.Close()
	apiContent, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read restored API response: %v", err)
	}
	if response.StatusCode != http.StatusOK || sha256.Sum256(apiContent) != sha256.Sum256(activeContent) {
		t.Fatalf("restored API download status=%d hash_matches=%t", response.StatusCode, sha256.Sum256(apiContent) == sha256.Sum256(activeContent))
	}
}

func TestVerifyCommandRejectsMigrationManifestMismatch(t *testing.T) {
	t.Parallel()

	backupDir := createBackupFixture(t, "migration.txt", "migration")
	manifestPath := filepath.Join(backupDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var manifest admin.BackupManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest.DBSchemaMigrations = []string{}
	manifestBytes, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	if err := os.WriteFile(manifestPath, append(manifestBytes, '\n'), 0o644); err != nil {
		t.Fatalf("rewrite manifest: %v", err)
	}
	var output bytes.Buffer
	exitCode := run(context.Background(), []string{"verify", "--backup", backupDir}, &output)
	if exitCode != 1 {
		t.Fatalf("verify exit code = %d, want 1; output = %s", exitCode, output.String())
	}
	if !strings.Contains(output.String(), "migration manifest mismatch") {
		t.Fatalf("verify output does not identify migration mismatch: %s", output.String())
	}
}

func TestRestoreVerificationFailurePreservesOriginalTarget(t *testing.T) {
	t.Parallel()

	backupDir := createBackupFixture(t, "corrupt.txt", "original backup")
	if err := os.WriteFile(filepath.Join(backupDir, "files", "fixture-file", "corrupt.txt"), []byte("corrupted data"), 0o644); err != nil {
		t.Fatalf("corrupt backup object: %v", err)
	}
	targetBase := t.TempDir()
	targetRoot := filepath.Join(targetBase, "data", "files")
	targetDB := filepath.Join(targetBase, "data", "db", "memodrive.db")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatalf("create original target: %v", err)
	}
	originalPath := filepath.Join(targetRoot, "keep.txt")
	if err := os.WriteFile(originalPath, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write original target: %v", err)
	}
	var output bytes.Buffer
	exitCode := run(context.Background(), []string{
		"restore", "--backup", backupDir, "--target-root", targetRoot, "--target-db", targetDB, "--force",
	}, &output)
	if exitCode != 1 {
		t.Fatalf("restore exit code = %d, want 1; output = %s", exitCode, output.String())
	}
	original, err := os.ReadFile(originalPath)
	if err != nil {
		t.Fatalf("read preserved target: %v", err)
	}
	if string(original) != "keep me" {
		t.Fatalf("original target changed to %q", original)
	}
}

func createBackupFixture(t *testing.T, name, content string) string {
	t.Helper()
	fixture := createSourceFixture(t, name, content)
	backupDir := filepath.Join(fixture.Workspace, "backup")
	var output bytes.Buffer
	if exitCode := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", fixture.EnvPath}, &output); exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, output.String())
	}
	return backupDir
}

type sourceFixture struct {
	Workspace string
	EnvPath   string
	Config    *config.Config
	File      *model.File
}

func createSourceFixture(t *testing.T, name, content string) sourceFixture {
	t.Helper()
	workspace := t.TempDir()
	storageRoot := filepath.Join(workspace, "data", "files")
	databasePath := filepath.Join(workspace, "data", "db", "memodrive.db")
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:         storageRoot,
		DBPath:       databasePath,
		TempDir:      filepath.Join(workspace, "data", "tmp"),
		ThumbnailDir: filepath.Join(workspace, "data", "thumbnails"),
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure fixture directories: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open fixture database: %v", err)
	}
	file := &model.File{
		ID:          "fixture-file",
		Name:        name,
		Path:        "/",
		StoragePath: filepath.Join("fixture-file", name),
		Size:        int64(len(content)),
		MimeType:    "text/plain",
		Status:      "uploaded",
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create fixture file: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	objectPath := filepath.Join(storageRoot, file.StoragePath)
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o755); err != nil {
		t.Fatalf("create fixture object directory: %v", err)
	}
	if err := os.WriteFile(objectPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture object: %v", err)
	}
	envPath := filepath.Join(workspace, "memodrive.env")
	if err := os.WriteFile(envPath, []byte(fmt.Sprintf(
		"STORAGE_ROOT=%s\nDB_PATH=%s\nUPLOAD_TEMP_DIR=%s\nTHUMBNAIL_DIR=%s\n",
		storageRoot,
		databasePath,
		cfg.Storage.TempDir,
		cfg.Storage.ThumbnailDir,
	)), 0o600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return sourceFixture{
		Workspace: workspace,
		EnvPath:   envPath,
		Config:    cfg,
		File:      file,
	}
}

func createThumbnailBackupFixture(t *testing.T) (string, string) {
	t.Helper()
	fixture := createSourceFixture(t, "photo.jpg", "photo")
	thumbnailName := "fixture-file.jpg"
	db, err := store.Open(context.Background(), fixture.Config)
	if err != nil {
		t.Fatalf("open fixture database for thumbnail: %v", err)
	}
	if err := db.UpsertMetadata(context.Background(), &model.FileMetadata{
		FileID:        fixture.File.ID,
		MetaJSON:      `{}`,
		ThumbnailPath: &thumbnailName,
	}); err != nil {
		t.Fatalf("create fixture thumbnail metadata: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close fixture database: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fixture.Config.Storage.ThumbnailDir, thumbnailName), []byte("thumbnail"), 0o644); err != nil {
		t.Fatalf("write fixture thumbnail: %v", err)
	}
	backupDir := filepath.Join(fixture.Workspace, "backup")
	var output bytes.Buffer
	if exitCode := run(context.Background(), []string{"backup", "--output", backupDir, "--env-file", fixture.EnvPath}, &output); exitCode != 0 {
		t.Fatalf("backup exit code = %d, output = %s", exitCode, output.String())
	}
	return backupDir, thumbnailName
}
