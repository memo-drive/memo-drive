// Package store provides SQLite-backed persistence for all domain entities.
// It manages schema migrations, file CRUD, chunk storage with full-text search,
// and conversation/message history.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

// Store wraps a SQLite database connection and provides all persistence operations.
type Store struct {
	db *sql.DB
}

// Open creates a new Store, applies pending migrations, and returns it ready for use.
func Open(ctx context.Context, cfg *config.Config) (*Store, error) {
	started := time.Now()
	db, err := sql.Open("sqlite3", fmt.Sprintf("%s?_foreign_keys=on&_busy_timeout=5000&_txlock=immediate&parseTime=true", cfg.Storage.DBPath))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	log.Printf("level=info component=store event=migration_complete duration_ms=%d", time.Since(started).Milliseconds())
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const baseSchema = `
CREATE TABLE IF NOT EXISTS files (
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
    last_viewed_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);

CREATE TABLE IF NOT EXISTS file_metadata (
    file_id TEXT PRIMARY KEY,
    meta_json TEXT NOT NULL,
    thumbnail_path TEXT,
    extracted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tasks (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    type TEXT NOT NULL,
    status TEXT DEFAULT 'pending',
    progress INTEGER DEFAULT 0,
    error TEXT,
    retry_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS conversations (
    id TEXT PRIMARY KEY,
    title TEXT,
    mode TEXT CHECK(mode IN ('rag', 'file_qa', 'search')),
    file_ids TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS messages (
    id TEXT PRIMARY KEY,
    conversation_id TEXT NOT NULL,
    role TEXT CHECK(role IN ('user', 'assistant')),
    content TEXT NOT NULL,
    sources TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS upload_sessions (
    id TEXT PRIMARY KEY,
    file_name TEXT NOT NULL,
    file_size INTEGER NOT NULL,
    chunk_size INTEGER NOT NULL,
    uploaded_chunks TEXT DEFAULT '[]',
    dest_path TEXT NOT NULL,
    status TEXT DEFAULT 'uploading',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    expires_at DATETIME NOT NULL
);`
	if _, err := s.db.ExecContext(ctx, baseSchema); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
    id TEXT PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "010_trash_columns", `
ALTER TABLE files ADD COLUMN deleted_at DATETIME;
ALTER TABLE files ADD COLUMN original_path TEXT;
ALTER TABLE files ADD COLUMN original_name TEXT;
CREATE INDEX IF NOT EXISTS idx_files_deleted_at ON files(deleted_at);
`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "011_trash_root_id", `
ALTER TABLE files ADD COLUMN trash_root_id TEXT;
CREATE INDEX IF NOT EXISTS idx_files_trash_root_id ON files(trash_root_id);
`); err != nil {
		return err
	}
	if err := s.migrateConversationColumns(ctx); err != nil {
		return err
	}
	if err := s.migrateChunks(ctx); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "014_last_viewed_at", `
ALTER TABLE files ADD COLUMN last_viewed_at DATETIME;
`); err != nil {
		return err
	}
	if err := s.migrateActiveFilePathUnique(ctx); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "016_upload_conflict_policy", `
ALTER TABLE upload_sessions ADD COLUMN overwrite_policy TEXT NOT NULL DEFAULT 'reject';
ALTER TABLE upload_sessions ADD COLUMN resolved_name TEXT;
ALTER TABLE upload_sessions ADD COLUMN existing_file_id TEXT;
`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "017_file_mutations", `
CREATE TABLE IF NOT EXISTS file_mutations (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    state TEXT NOT NULL,
    virtual_path TEXT NOT NULL,
    target_file_id TEXT,
    staged_path TEXT NOT NULL,
    old_storage_path TEXT,
    final_storage_path TEXT,
    error TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_mutations_state_updated
ON file_mutations(state, updated_at);
`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "018_auth_sessions", `
CREATE TABLE IF NOT EXISTS auth_sessions (
    id TEXT PRIMARY KEY,
    subject TEXT NOT NULL,
    credential_fingerprint TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL,
    revoked_at DATETIME
);
CREATE INDEX IF NOT EXISTS idx_auth_sessions_subject_active
ON auth_sessions(subject, revoked_at, expires_at);
	`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "019_file_copy_operations", `
CREATE TABLE IF NOT EXISTS file_copy_operations (
    id TEXT PRIMARY KEY,
    source_file_id TEXT NOT NULL,
    root_file_id TEXT,
    state TEXT NOT NULL,
    error TEXT,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_file_copy_operations_state_updated
ON file_copy_operations(state, updated_at);
	`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "020_file_versions", `
CREATE TABLE IF NOT EXISTS file_versions (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    version_no INTEGER NOT NULL,
    storage_path TEXT NOT NULL,
    size INTEGER NOT NULL,
    mime_type TEXT,
    sha256 TEXT,
    source TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE,
    UNIQUE(file_id, version_no)
);
CREATE INDEX IF NOT EXISTS idx_file_versions_file_created
ON file_versions(file_id, created_at DESC);
	`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "021_task_retries", `
ALTER TABLE tasks ADD COLUMN retry_of_task_id TEXT REFERENCES tasks(id);
CREATE INDEX IF NOT EXISTS idx_tasks_created_id
ON tasks(created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_status_file_created
ON tasks(status, file_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_tasks_retry_of
ON tasks(retry_of_task_id);
	`); err != nil {
		return err
	}
	if err := s.applyOnce(ctx, "022_active_task_unique", `
WITH ranked AS (
    SELECT id,
           ROW_NUMBER() OVER (PARTITION BY file_id ORDER BY created_at DESC, id DESC) AS active_rank
    FROM tasks
    WHERE status IN ('pending', 'processing')
)
UPDATE tasks
SET status = 'failed',
    progress = 100,
    error = CASE
        WHEN error IS NULL OR error = '' THEN 'superseded during active Task uniqueness migration'
        ELSE error
    END,
    updated_at = CURRENT_TIMESTAMP
WHERE id IN (SELECT id FROM ranked WHERE active_rank > 1);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_active_file
ON tasks(file_id)
WHERE status IN ('pending', 'processing');
	`); err != nil {
		return err
	}
	return nil
}

func (s *Store) migrateActiveFilePathUnique(ctx context.Context) error {
	const migrationID = "015_active_file_path_unique"
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE id = ?`, migrationID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var pathValue string
	var lowerName string
	var ids string
	err = s.db.QueryRowContext(ctx, `
SELECT path, lower(name), group_concat(id, ',')
FROM files
WHERE deleted_at IS NULL
GROUP BY path, lower(name)
HAVING COUNT(*) > 1
LIMIT 1`).Scan(&pathValue, &lowerName, &ids)
	if err == nil {
		return fmt.Errorf("migration %s duplicate active file path: path=%q lower_name=%q ids=%s", migrationID, pathValue, lowerName, ids)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if _, err := s.db.ExecContext(ctx, `
CREATE UNIQUE INDEX IF NOT EXISTS idx_files_active_path_lower_name
ON files(path, lower(name))
WHERE deleted_at IS NULL;
`); err != nil {
		return fmt.Errorf("migration %s create unique index: %w", migrationID, err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO schema_migrations(id) VALUES(?)`, migrationID)
	return err
}

func (s *Store) migrateChunks(ctx context.Context) error {
	const migrationID = "013_chunks_fts"
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE id = ?`, migrationID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	const base = `
CREATE TABLE IF NOT EXISTS chunks (
    id TEXT PRIMARY KEY,
    file_id TEXT NOT NULL,
    file_name TEXT NOT NULL DEFAULT '',
    heading TEXT NOT NULL DEFAULT '',
    chunk_index INTEGER NOT NULL DEFAULT 0,
    text TEXT NOT NULL DEFAULT '',
    parent_chunk_id TEXT NOT NULL DEFAULT '',
    is_parent BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (file_id) REFERENCES files(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_chunks_file_id ON chunks(file_id);
CREATE INDEX IF NOT EXISTS idx_chunks_parent_chunk_id ON chunks(parent_chunk_id);
`
	if _, err := s.db.ExecContext(ctx, base); err != nil {
		return fmt.Errorf("migration %s create chunks: %w", migrationID, err)
	}

	const fts5Script = `
CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
    text,
    content='chunks',
    content_rowid='rowid',
    tokenize='unicode61'
);

CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
    INSERT INTO chunks_fts(rowid, text) VALUES (new.rowid, new.text);
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', old.rowid, old.text);
END;

CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
    INSERT INTO chunks_fts(chunks_fts, rowid, text) VALUES('delete', old.rowid, old.text);
    INSERT INTO chunks_fts(rowid, text) VALUES (new.rowid, new.text);
END;
`
	if _, err := s.db.ExecContext(ctx, fts5Script); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "no such module: fts5") {
			return fmt.Errorf("migration %s create fts5: %w", migrationID, err)
		}
		log.Printf("level=warn component=store event=chunks_fts5_unavailable fallback=like err=%q", err)
		if _, fallbackErr := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS chunks_fts (
    rowid INTEGER PRIMARY KEY,
    text TEXT NOT NULL DEFAULT ''
);

CREATE TRIGGER IF NOT EXISTS chunks_ai AFTER INSERT ON chunks BEGIN
    INSERT OR REPLACE INTO chunks_fts(rowid, text) VALUES (new.rowid, new.text);
END;

CREATE TRIGGER IF NOT EXISTS chunks_ad AFTER DELETE ON chunks BEGIN
    DELETE FROM chunks_fts WHERE rowid = old.rowid;
END;

CREATE TRIGGER IF NOT EXISTS chunks_au AFTER UPDATE ON chunks BEGIN
    INSERT OR REPLACE INTO chunks_fts(rowid, text) VALUES (new.rowid, new.text);
END;
`); fallbackErr != nil {
			return fmt.Errorf("migration %s create fallback fts table: %w", migrationID, fallbackErr)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO schema_migrations(id) VALUES(?)`, migrationID)
	return err
}

func (s *Store) migrateConversationColumns(ctx context.Context) error {
	const migrationID = "012_conversation_columns"
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE id = ?`, migrationID).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if !s.columnExists(ctx, "conversations", "updated_at") {
		if _, err := s.db.ExecContext(ctx, `ALTER TABLE conversations ADD COLUMN updated_at DATETIME`); err != nil {
			return fmt.Errorf("migration %s add updated_at: %w", migrationID, err)
		}
	}
	if _, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET updated_at = COALESCE(updated_at, created_at, CURRENT_TIMESTAMP)
WHERE updated_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
`); err != nil {
		return fmt.Errorf("migration %s indexes: %w", migrationID, err)
	}
	_, err = s.db.ExecContext(ctx, `INSERT OR IGNORE INTO schema_migrations(id) VALUES(?)`, migrationID)
	return err
}

func (s *Store) applyOnce(ctx context.Context, id, sqlText string) error {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE id = ?`, id).Scan(&exists)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	for _, stmt := range strings.Split(sqlText, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		fields := strings.Fields(stmt)
		if len(fields) >= 6 &&
			strings.EqualFold(fields[0], "ALTER") &&
			strings.EqualFold(fields[1], "TABLE") &&
			strings.EqualFold(fields[3], "ADD") &&
			strings.EqualFold(fields[4], "COLUMN") {
			table := strings.Trim(fields[2], "`\"[]")
			column := strings.Trim(fields[5], "`\"[]")
			if s.columnExists(ctx, table, column) {
				continue
			}
		}
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration %s exec %q: %w", id, stmt, err)
		}
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO schema_migrations(id) VALUES(?)`, id)
	return err
}

func (s *Store) columnExists(ctx context.Context, table, column string) bool {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`PRAGMA table_info(%s)`, table))
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false
		}
		if name == column {
			return true
		}
	}
	return false
}
