package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
)

func TestExistingUploadSessionMigratesToRejectPolicy(t *testing.T) {
	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "memodrive.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	legacy, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if _, err := legacy.Exec(`
CREATE TABLE files (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    path TEXT NOT NULL,
    storage_path TEXT NOT NULL,
    size INTEGER,
    mime_type TEXT,
    is_dir BOOLEAN DEFAULT FALSE,
    parent_id TEXT,
    status TEXT DEFAULT 'uploaded',
    chunk_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_viewed_at DATETIME,
    deleted_at DATETIME,
    original_path TEXT,
    original_name TEXT,
    trash_root_id TEXT
);
CREATE TABLE upload_sessions (
    id TEXT PRIMARY KEY,
    file_name TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    chunk_size INTEGER NOT NULL,
    uploaded_chunks TEXT DEFAULT '[]',
    dest_path TEXT NOT NULL,
    status TEXT DEFAULT 'uploading',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);
CREATE TABLE schema_migrations (
    id TEXT PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO schema_migrations(id) VALUES
    ('010_trash_columns'),
    ('011_trash_root_id'),
    ('012_conversation_columns'),
    ('013_chunks_fts'),
    ('014_last_viewed_at'),
    ('015_active_file_path_unique');
INSERT INTO upload_sessions (
    id, file_name, file_size, chunk_size, uploaded_chunks, dest_path, status,
    created_at, expires_at
) VALUES (
    'legacy-upload', 'legacy.pdf', 128, 64, '[0]', '/', 'uploading',
    '2026-07-28 00:00:00', '2026-07-29 00:00:00'
);
`); err != nil {
		_ = legacy.Close()
		t.Fatalf("seed legacy database: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(context.Background(), &config.Config{
		Storage: config.StorageConfig{DBPath: dbPath},
	})
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer db.Close()

	session, err := db.GetUploadSession(context.Background(), "legacy-upload")
	if err != nil {
		t.Fatalf("get migrated upload session: %v", err)
	}
	if session.OverwritePolicy != "reject" {
		t.Fatalf("expected legacy policy reject, got %q", session.OverwritePolicy)
	}
	if session.RequestedName != "legacy.pdf" || session.ResolvedName != "legacy.pdf" {
		t.Fatalf("unexpected migrated target requested=%q resolved=%q", session.RequestedName, session.ResolvedName)
	}
	if session.ExistingFileID != "" {
		t.Fatalf("expected no existing File target, got %q", session.ExistingFileID)
	}
}

func TestUploadConflictMigrationRecoversWhenColumnsExistWithoutMarker(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			DBPath: filepath.Join(root, "db", "memodrive.db"),
		},
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.DBPath), 0o755); err != nil {
		t.Fatalf("create database directory: %v", err)
	}

	first, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	raw, err := sql.Open("sqlite3", cfg.Storage.DBPath)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	if _, err := raw.Exec(`DELETE FROM schema_migrations WHERE id = '016_upload_conflict_policy'`); err != nil {
		_ = raw.Close()
		t.Fatalf("remove migration marker: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close migration database: %v", err)
	}

	reopened, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("reopen partially migrated store: %v", err)
	}
	defer reopened.Close()
}
