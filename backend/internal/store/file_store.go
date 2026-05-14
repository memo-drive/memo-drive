package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/model"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

const (
	defaultFileSearchLimit = 50
	maxFileSearchLimit     = 500
	metadataSnippetRadius  = 80
	fileColumns            = "id, name, path, storage_path, size, mime_type, is_dir, parent_id, status, chunk_count, created_at, updated_at, deleted_at, original_path, original_name, trash_root_id"
	fileColumnsWithAlias   = "f.id, f.name, f.path, f.storage_path, f.size, f.mime_type, f.is_dir, f.parent_id, f.status, f.chunk_count, f.created_at, f.updated_at, f.deleted_at, f.original_path, f.original_name, f.trash_root_id"
)

// FileSearchFilter holds optional criteria for filtering file queries.
type FileSearchFilter struct {
	Keyword    string
	PathPrefix string
	MimePrefix string
	Extensions []string
	DateFrom   *time.Time
	DateTo     *time.Time
	Limit      int
}

func (f FileSearchFilter) HasStructuredFilters() bool {
	return strings.TrimSpace(f.MimePrefix) != "" || len(f.Extensions) > 0 || f.DateFrom != nil || f.DateTo != nil
}

// MetadataHit pairs a file with a snippet of matched metadata text.
type MetadataHit struct {
	File    model.File
	Snippet string
}

func (s *Store) CreateFile(ctx context.Context, file *model.File) error {
	now := time.Now().UTC()
	file.CreatedAt = now
	file.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO files (id, name, path, storage_path, size, mime_type, is_dir, parent_id, status, chunk_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.ID,
		file.Name,
		file.Path,
		file.StoragePath,
		file.Size,
		file.MimeType,
		file.IsDir,
		nullableString(file.ParentID),
		file.Status,
		file.ChunkCount,
		file.CreatedAt,
		file.UpdatedAt,
	)
	return err
}

func (s *Store) ListFiles(ctx context.Context, dirPath, sort string) ([]model.File, error) {
	orderBy := "is_dir DESC, name COLLATE NOCASE ASC"
	switch sort {
	case "size":
		orderBy = "is_dir DESC, size DESC, name COLLATE NOCASE ASC"
	case "created_at":
		orderBy = "created_at DESC"
	case "updated_at":
		orderBy = "updated_at DESC"
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files
WHERE path = ?
  AND deleted_at IS NULL
ORDER BY %s`, fileColumns, orderBy), dirPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) SearchFilesByName(ctx context.Context, filter FileSearchFilter) ([]model.File, error) {
	filter = normalizeFileSearchFilter(filter)
	if filter.Keyword == "" {
		return nil, nil
	}
	clauses, args := fileFilterClauses(filter, "")
	query := fmt.Sprintf(`
SELECT %s
FROM files
WHERE is_dir = 0
  AND deleted_at IS NULL
  AND name LIKE '%%' || ? || '%%' COLLATE NOCASE
  %s
ORDER BY updated_at DESC
LIMIT ?`, fileColumns, clauses)
	args = append([]any{filter.Keyword}, args...)
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) SearchFilesByMetadata(ctx context.Context, filter FileSearchFilter) ([]MetadataHit, error) {
	filter = normalizeFileSearchFilter(filter)
	if filter.Keyword == "" {
		return nil, nil
	}
	clauses, args := fileFilterClauses(filter, "f")
	query := fmt.Sprintf(`
SELECT %s, fm.meta_json
FROM files f
JOIN file_metadata fm ON fm.file_id = f.id
WHERE f.is_dir = 0
  AND f.deleted_at IS NULL
  AND fm.meta_json LIKE '%%' || ? || '%%' COLLATE NOCASE
  %s
ORDER BY f.updated_at DESC
LIMIT ?`, fileColumnsWithAlias, clauses)
	args = append([]any{filter.Keyword}, args...)
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hits []MetadataHit
	for rows.Next() {
		hit, err := scanMetadataHit(rows, filter.Keyword)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	return hits, rows.Err()
}

func (s *Store) ListFileIDsByFilter(ctx context.Context, filter FileSearchFilter) ([]string, error) {
	filter = normalizeFileSearchFilter(filter)
	clauses, args := fileFilterClauses(filter, "")
	query := fmt.Sprintf(`
SELECT id
FROM files
WHERE is_dir = 0
  AND deleted_at IS NULL
  %s
ORDER BY created_at DESC
LIMIT ?`, clauses)
	args = append(args, filter.Limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) TotalActiveFileSize(ctx context.Context) (int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx, `
SELECT COALESCE(SUM(size), 0)
FROM files
WHERE is_dir = 0
  AND deleted_at IS NULL`).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) ListDescendants(ctx context.Context, virtualPath string) ([]model.File, error) {
	virtualPath = cleanFileSearchPath(virtualPath)
	if virtualPath == "" {
		virtualPath = "/"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files
WHERE deleted_at IS NULL
  AND (path = ? OR path LIKE ? || '/%%')
ORDER BY length(path) ASC, path ASC, name COLLATE NOCASE ASC`, fileColumns),
		virtualPath,
		virtualPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) ListTrashedDescendants(ctx context.Context, originalVirtualPath string) ([]model.File, error) {
	originalVirtualPath = cleanFileSearchPath(originalVirtualPath)
	if originalVirtualPath == "" {
		originalVirtualPath = "/"
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files
WHERE deleted_at IS NOT NULL
  AND original_path IS NOT NULL
  AND (original_path = ? OR original_path LIKE ? || '/%%')
ORDER BY length(original_path) ASC, original_path ASC, original_name COLLATE NOCASE ASC`, fileColumns),
		originalVirtualPath,
		originalVirtualPath,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) ExistsAtPath(ctx context.Context, dirPath, name string) (bool, error) {
	dirPath = cleanFileSearchPath(dirPath)
	if dirPath == "" {
		dirPath = "/"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return false, nil
	}
	var exists int
	if err := s.db.QueryRowContext(ctx, `
SELECT 1
FROM files
WHERE path = ? AND name = ? COLLATE NOCASE AND deleted_at IS NULL
LIMIT 1`, dirPath, name).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) GetFile(ctx context.Context, id string) (*model.File, error) {
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files
WHERE id = ? AND deleted_at IS NULL`, fileColumns), id)
	file, err := scanFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *Store) FileExists(ctx context.Context, id string) (bool, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `SELECT 1 FROM files WHERE id = ? AND deleted_at IS NULL LIMIT 1`, id).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) ListFilesByStatus(ctx context.Context, status string) ([]model.File, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files
WHERE status = ? AND deleted_at IS NULL`, fileColumns), status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) ListStoragePaths(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT storage_path FROM files WHERE is_dir = 0`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := map[string]struct{}{}
	for rows.Next() {
		var storagePath string
		if err := rows.Scan(&storagePath); err != nil {
			return nil, err
		}
		paths[storagePath] = struct{}{}
	}
	return paths, rows.Err()
}

func (s *Store) UpdateFileStatus(ctx context.Context, id, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE files SET status = ?, updated_at = ? WHERE id = ?`, status, time.Now().UTC(), id)
	return affected(result, err)
}

func (s *Store) UpdateFileChunkCount(ctx context.Context, id string, chunkCount int) error {
	result, err := s.db.ExecContext(ctx, `UPDATE files SET chunk_count = ?, updated_at = ? WHERE id = ?`, chunkCount, time.Now().UTC(), id)
	return affected(result, err)
}

func (s *Store) UpdateFileLocation(ctx context.Context, file *model.File) error {
	file.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE files
SET name = ?, path = ?, storage_path = ?, updated_at = ?
WHERE id = ?`,
		file.Name,
		file.Path,
		file.StoragePath,
		file.UpdatedAt,
		file.ID,
	)
	return affected(result, err)
}

func (s *Store) UpdateFilePath(ctx context.Context, id, newPath, newStoragePath string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE files
SET path = ?, storage_path = ?, updated_at = ?
WHERE id = ?`, newPath, newStoragePath, time.Now().UTC(), id)
	return affected(result, err)
}

func (s *Store) SoftDeleteFile(ctx context.Context, id, trashRootID string) error {
	trashRootID = strings.TrimSpace(trashRootID)
	if trashRootID == "" {
		trashRootID = id
	}
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE files
SET deleted_at = ?,
    original_path = COALESCE(original_path, path),
    original_name = COALESCE(original_name, name),
    trash_root_id = ?,
    path = '/.trash',
    name = id || '-' || name,
    updated_at = ?
WHERE id = ? AND deleted_at IS NULL`, now, trashRootID, now, id)
	return affected(result, err)
}

func (s *Store) RestoreFile(ctx context.Context, id, fallbackPath, fallbackName string) error {
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `
UPDATE files
SET deleted_at = NULL,
    path = ?,
    name = ?,
    original_path = NULL,
    original_name = NULL,
    trash_root_id = NULL,
    updated_at = ?
WHERE id = ? AND deleted_at IS NOT NULL`, fallbackPath, fallbackName, now, id)
	return affected(result, err)
}

func (s *Store) ListTrashed(ctx context.Context, limit int) ([]model.File, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files f
WHERE f.deleted_at IS NOT NULL
  AND %s
ORDER BY f.deleted_at DESC
LIMIT ?`, fileColumnsWithAlias, trashRootPredicate("f")), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) ListExpiredTrashed(ctx context.Context, before time.Time) ([]model.File, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files f
WHERE f.deleted_at IS NOT NULL
  AND f.deleted_at < ?
  AND %s
ORDER BY f.deleted_at ASC`, fileColumnsWithAlias, trashRootPredicate("f")), before.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var files []model.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

func (s *Store) GetFileIncludeDeleted(ctx context.Context, id string) (*model.File, error) {
	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT %s
FROM files
WHERE id = ?`, fileColumns), id)
	file, err := scanFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &file, nil
}

func (s *Store) PurgeFile(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM files WHERE id = ?`, id)
	return affected(result, err)
}

func (s *Store) UpsertMetadata(ctx context.Context, metadata *model.FileMetadata) error {
	metadata.ExtractedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
INSERT INTO file_metadata (file_id, meta_json, thumbnail_path, extracted_at)
VALUES (?, ?, ?, ?)
ON CONFLICT(file_id) DO UPDATE SET
  meta_json = excluded.meta_json,
  thumbnail_path = excluded.thumbnail_path,
  extracted_at = excluded.extracted_at`,
		metadata.FileID,
		metadata.MetaJSON,
		nullableString(metadata.ThumbnailPath),
		metadata.ExtractedAt,
	)
	return err
}

func (s *Store) GetMetadata(ctx context.Context, fileID string) (*model.FileMetadata, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT file_id, meta_json, thumbnail_path, extracted_at
FROM file_metadata
WHERE file_id = ?`, fileID)
	var meta model.FileMetadata
	var thumb sql.NullString
	if err := row.Scan(&meta.FileID, &meta.MetaJSON, &thumb, &meta.ExtractedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if thumb.Valid {
		meta.ThumbnailPath = &thumb.String
	}
	return &meta, nil
}

func (s *Store) CreateTask(ctx context.Context, task *model.Task) error {
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO tasks (id, file_id, type, status, progress, error, retry_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		task.ID,
		task.FileID,
		task.Type,
		task.Status,
		task.Progress,
		nullableString(task.Error),
		task.RetryCount,
		task.CreatedAt,
		task.UpdatedAt,
	)
	return err
}

func (s *Store) UpdateTask(ctx context.Context, id, status string, progress int, errText *string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET status = ?, progress = ?, error = ?, updated_at = ?
WHERE id = ?`, status, progress, nullableString(errText), time.Now().UTC(), id)
	return affected(result, err)
}

func (s *Store) MarkTaskFailed(ctx context.Context, id string, msg string) error {
	return s.UpdateTask(ctx, id, model.TaskStatusFailed, 100, &msg)
}

func (s *Store) IncrementTaskRetry(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE tasks
SET retry_count = retry_count + 1, updated_at = ?
WHERE id = ?`, time.Now().UTC(), id)
	return affected(result, err)
}

func (s *Store) ListStuckTasks(ctx context.Context, olderThan time.Time) ([]model.Task, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, file_id, type, status, progress, error, retry_count, created_at, updated_at
FROM tasks
WHERE status IN (?, ?) AND updated_at < ?
ORDER BY updated_at ASC`, model.TaskStatusPending, model.TaskStatusProcessing, olderThan.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []model.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func (s *Store) HasActiveTaskForFile(ctx context.Context, fileID string) (bool, error) {
	var exists int
	if err := s.db.QueryRowContext(ctx, `
SELECT 1
FROM tasks
WHERE file_id = ? AND status IN (?, ?)
LIMIT 1`, fileID, model.TaskStatusPending, model.TaskStatusProcessing).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (s *Store) GetTask(ctx context.Context, id string) (*model.Task, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, file_id, type, status, progress, error, retry_count, created_at, updated_at
FROM tasks
WHERE id = ?`, id)
	task, err := scanTask(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &task, nil
}

func (s *Store) CreateUploadSession(ctx context.Context, session *model.UploadSession) error {
	chunks, err := json.Marshal(session.UploadedChunks)
	if err != nil {
		return err
	}
	session.CreatedAt = time.Now().UTC()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO upload_sessions (id, file_name, file_size, chunk_size, uploaded_chunks, dest_path, status, created_at, expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID,
		session.FileName,
		session.FileSize,
		session.ChunkSize,
		string(chunks),
		session.DestPath,
		session.Status,
		session.CreatedAt,
		session.ExpiresAt,
	)
	return err
}

func (s *Store) GetUploadSession(ctx context.Context, id string) (*model.UploadSession, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, file_name, file_size, chunk_size, uploaded_chunks, dest_path, status, created_at, expires_at
FROM upload_sessions
WHERE id = ?`, id)
	return scanUploadSession(row)
}

func (s *Store) ListUploadSessions(ctx context.Context, limit int) ([]model.UploadSession, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, file_name, file_size, chunk_size, uploaded_chunks, dest_path, status, created_at, expires_at
FROM upload_sessions
ORDER BY created_at DESC
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.UploadSession
	for rows.Next() {
		session, err := scanUploadSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, *session)
	}
	return sessions, rows.Err()
}

func (s *Store) UpdateUploadChunks(ctx context.Context, id string, chunks []int) error {
	encoded, err := json.Marshal(chunks)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE upload_sessions SET uploaded_chunks = ? WHERE id = ?`, string(encoded), id)
	return affected(result, err)
}

func (s *Store) UpdateUploadStatus(ctx context.Context, id, status string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE upload_sessions SET status = ? WHERE id = ?`, status, id)
	return affected(result, err)
}

func (s *Store) DeleteUploadSession(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM upload_sessions WHERE id = ?`, id)
	return affected(result, err)
}

func (s *Store) ClearUploadSessions(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id
FROM upload_sessions
WHERE status NOT IN ('uploading', 'merging')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ids, nil
	}
	quoted := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		quoted[i] = "?"
		args[i] = id
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM upload_sessions WHERE id IN (%s)`, strings.Join(quoted, ",")), args...)
	return ids, err
}

func (s *Store) DeleteExpiredUploadSessions(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM upload_sessions WHERE status = 'uploading' AND expires_at < ?`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ids, nil
	}
	quoted := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		quoted[i] = "?"
		args[i] = id
	}
	_, err = s.db.ExecContext(ctx, fmt.Sprintf(`UPDATE upload_sessions SET status = 'expired' WHERE id IN (%s)`, strings.Join(quoted, ",")), args...)
	return ids, err
}

func scanUploadSession(scanner fileScanner) (*model.UploadSession, error) {
	var session model.UploadSession
	var chunks string
	if err := scanner.Scan(
		&session.ID,
		&session.FileName,
		&session.FileSize,
		&session.ChunkSize,
		&chunks,
		&session.DestPath,
		&session.Status,
		&session.CreatedAt,
		&session.ExpiresAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := json.Unmarshal([]byte(chunks), &session.UploadedChunks); err != nil {
		return nil, err
	}
	return &session, nil
}

type fileScanner interface {
	Scan(dest ...any) error
}

func scanFile(scanner fileScanner) (model.File, error) {
	var file model.File
	var parent sql.NullString
	var deletedAt sql.NullTime
	var originalPath sql.NullString
	var originalName sql.NullString
	var trashRootID sql.NullString
	if err := scanner.Scan(
		&file.ID,
		&file.Name,
		&file.Path,
		&file.StoragePath,
		&file.Size,
		&file.MimeType,
		&file.IsDir,
		&parent,
		&file.Status,
		&file.ChunkCount,
		&file.CreatedAt,
		&file.UpdatedAt,
		&deletedAt,
		&originalPath,
		&originalName,
		&trashRootID,
	); err != nil {
		return file, err
	}
	if parent.Valid {
		file.ParentID = &parent.String
	}
	applyTrashFields(&file, deletedAt, originalPath, originalName, trashRootID)
	return file, nil
}

func scanMetadataHit(scanner fileScanner, keyword string) (MetadataHit, error) {
	var hit MetadataHit
	var metaJSON string
	var parent sql.NullString
	var deletedAt sql.NullTime
	var originalPath sql.NullString
	var originalName sql.NullString
	var trashRootID sql.NullString
	if err := scanner.Scan(
		&hit.File.ID,
		&hit.File.Name,
		&hit.File.Path,
		&hit.File.StoragePath,
		&hit.File.Size,
		&hit.File.MimeType,
		&hit.File.IsDir,
		&parent,
		&hit.File.Status,
		&hit.File.ChunkCount,
		&hit.File.CreatedAt,
		&hit.File.UpdatedAt,
		&deletedAt,
		&originalPath,
		&originalName,
		&trashRootID,
		&metaJSON,
	); err != nil {
		return hit, err
	}
	if parent.Valid {
		hit.File.ParentID = &parent.String
	}
	applyTrashFields(&hit.File, deletedAt, originalPath, originalName, trashRootID)
	hit.Snippet = makeMetadataSnippet(metaJSON, keyword)
	return hit, nil
}

func applyTrashFields(file *model.File, deletedAt sql.NullTime, originalPath, originalName, trashRootID sql.NullString) {
	if deletedAt.Valid {
		value := deletedAt.Time
		file.DeletedAt = &value
	}
	if originalPath.Valid {
		value := originalPath.String
		file.OriginalPath = &value
	}
	if originalName.Valid {
		value := originalName.String
		file.OriginalName = &value
	}
	if trashRootID.Valid {
		value := trashRootID.String
		file.TrashRootID = &value
	}
}

func scanTask(scanner fileScanner) (model.Task, error) {
	var task model.Task
	var errText sql.NullString
	if err := scanner.Scan(
		&task.ID,
		&task.FileID,
		&task.Type,
		&task.Status,
		&task.Progress,
		&errText,
		&task.RetryCount,
		&task.CreatedAt,
		&task.UpdatedAt,
	); err != nil {
		return task, err
	}
	if errText.Valid {
		task.Error = &errText.String
	}
	return task, nil
}

func normalizeFileSearchFilter(filter FileSearchFilter) FileSearchFilter {
	filter.Keyword = strings.TrimSpace(filter.Keyword)
	filter.PathPrefix = cleanFileSearchPath(filter.PathPrefix)
	filter.MimePrefix = strings.TrimSpace(filter.MimePrefix)
	filter.Extensions = normalizeFileExtensions(filter.Extensions)
	if filter.Limit <= 0 {
		filter.Limit = defaultFileSearchLimit
	}
	if filter.Limit > maxFileSearchLimit {
		filter.Limit = maxFileSearchLimit
	}
	return filter
}

func fileFilterClauses(filter FileSearchFilter, alias string) (string, []any) {
	column := func(name string) string {
		if alias == "" {
			return name
		}
		return alias + "." + name
	}
	var clauses []string
	var args []any
	if filter.PathPrefix != "" {
		clauses = append(clauses, fmt.Sprintf("(%s = ? OR %s LIKE ? || '/%%')", column("path"), column("path")))
		args = append(args, filter.PathPrefix, filter.PathPrefix)
	}
	if filter.MimePrefix != "" {
		clauses = append(clauses, fmt.Sprintf("IFNULL(%s, '') LIKE ? || '%%'", column("mime_type")))
		args = append(args, filter.MimePrefix)
	}
	if len(filter.Extensions) > 0 {
		extClauses := make([]string, 0, len(filter.Extensions))
		for _, ext := range filter.Extensions {
			extClauses = append(extClauses, fmt.Sprintf("LOWER(%s) LIKE ?", column("name")))
			args = append(args, "%."+strings.ToLower(ext))
		}
		clauses = append(clauses, "("+strings.Join(extClauses, " OR ")+")")
	}
	if filter.DateFrom != nil {
		clauses = append(clauses, fmt.Sprintf("%s >= ?", column("created_at")))
		args = append(args, filter.DateFrom.UTC())
	}
	if filter.DateTo != nil {
		clauses = append(clauses, fmt.Sprintf("%s <= ?", column("created_at")))
		args = append(args, filter.DateTo.UTC())
	}
	if len(clauses) == 0 {
		return "", args
	}
	return "AND " + strings.Join(clauses, "\n  AND "), args
}

func normalizeFileExtensions(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), ".")
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func cleanFileSearchPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return strings.TrimRight(value, "/")
}

func makeMetadataSnippet(metaJSON, keyword string) string {
	metaJSON = strings.Join(strings.Fields(metaJSON), " ")
	if metaJSON == "" {
		return ""
	}
	metaRunes := []rune(metaJSON)
	keywordRunes := []rune(strings.ToLower(strings.TrimSpace(keyword)))
	matchStart := indexRuneSequence([]rune(strings.ToLower(metaJSON)), keywordRunes)
	if len(keywordRunes) == 0 || matchStart < 0 {
		if len(metaRunes) <= metadataSnippetRadius*2 {
			return metaJSON
		}
		return string(metaRunes[:metadataSnippetRadius*2]) + "..."
	}
	matchEnd := minStoreInt(len(metaRunes), matchStart+len(keywordRunes))
	start := maxStoreInt(0, matchStart-metadataSnippetRadius)
	end := minStoreInt(len(metaRunes), matchEnd+metadataSnippetRadius)
	snippet := string(metaRunes[start:end])
	if start > 0 {
		snippet = "..." + snippet
	}
	if end < len(metaRunes) {
		snippet += "..."
	}
	return snippet
}

func indexRuneSequence(value, needle []rune) int {
	if len(needle) == 0 || len(needle) > len(value) {
		return -1
	}
	for i := 0; i <= len(value)-len(needle); i++ {
		matched := true
		for j := range needle {
			if value[i+j] != needle[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func minStoreInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxStoreInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func trashRootPredicate(alias string) string {
	return fmt.Sprintf(`(
	%s.trash_root_id = %s.id
	OR (
		%s.trash_root_id IS NULL
		AND NOT EXISTS (
			SELECT 1
			FROM files parent
			WHERE parent.deleted_at IS NOT NULL
			  AND parent.is_dir = 1
			  AND parent.id <> %s.id
			  AND parent.original_path IS NOT NULL
			  AND parent.original_name IS NOT NULL
			  AND (
				%s.original_path = CASE
					WHEN parent.original_path = '/' THEN '/' || parent.original_name
					ELSE parent.original_path || '/' || parent.original_name
				END
				OR %s.original_path LIKE (
					CASE
						WHEN parent.original_path = '/' THEN '/' || parent.original_name
						ELSE parent.original_path || '/' || parent.original_name
					END
				) || '/%%'
			  )
		)
	)
)`, alias, alias, alias, alias, alias, alias)
}

func nullableString(value *string) any {
	if value == nil || *value == "" {
		return nil
	}
	return *value
}

func affected(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
