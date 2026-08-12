package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/memodrive/backend/internal/model"
)

func (s *Store) CreateFileMutation(ctx context.Context, mutation *model.FileMutation) error {
	now := time.Now().UTC()
	mutation.CreatedAt = now
	mutation.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO file_mutations (
    id, kind, state, virtual_path, target_file_id, staged_path,
    old_storage_path, final_storage_path, error, created_at, updated_at
)
VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		mutation.ID,
		mutation.Kind,
		mutation.State,
		mutation.VirtualPath,
		mutation.TargetFileID,
		mutation.StagedPath,
		mutation.OldStoragePath,
		mutation.FinalStoragePath,
		mutation.Error,
		mutation.CreatedAt,
		mutation.UpdatedAt,
	)
	return err
}

func (s *Store) UpdateFileMutationState(ctx context.Context, id, state, errorText string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE file_mutations
SET state = ?, error = NULLIF(?, ''), updated_at = ?
WHERE id = ?`,
		state,
		errorText,
		time.Now().UTC(),
		id,
	)
	return affected(result, err)
}

func (s *Store) ListRecoverableFileMutations(ctx context.Context) ([]model.FileMutation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, kind, state, virtual_path, target_file_id, staged_path,
       old_storage_path, final_storage_path, error, created_at, updated_at
FROM file_mutations
WHERE state IN (?, ?, ?)
ORDER BY created_at ASC, id ASC`,
		model.FileMutationStatePrepared,
		model.FileMutationStateFSApplied,
		model.FileMutationStateDBCommitted,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mutations []model.FileMutation
	for rows.Next() {
		mutation, err := scanFileMutation(rows)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, mutation)
	}
	return mutations, rows.Err()
}

func (s *Store) ListTerminalFileMutationsBefore(
	ctx context.Context,
	olderThan time.Time,
	limit int,
) ([]model.FileMutation, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, kind, state, virtual_path, target_file_id, staged_path,
       old_storage_path, final_storage_path, error, created_at, updated_at
FROM file_mutations
WHERE state IN (?, ?) AND updated_at < ?
ORDER BY updated_at ASC, id ASC
LIMIT ?`,
		model.FileMutationStateFinalized,
		model.FileMutationStateFailed,
		olderThan.UTC(),
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mutations []model.FileMutation
	for rows.Next() {
		mutation, err := scanFileMutation(rows)
		if err != nil {
			return nil, err
		}
		mutations = append(mutations, mutation)
	}
	return mutations, rows.Err()
}

func (s *Store) DeleteTerminalFileMutation(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
DELETE FROM file_mutations
WHERE id = ? AND state IN (?, ?)`,
		id,
		model.FileMutationStateFinalized,
		model.FileMutationStateFailed,
	)
	return affected(result, err)
}

func (s *Store) ListFileMutationIDs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM file_mutations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}

// CommitFileMutation atomically publishes File metadata, completes its Upload
// Session, and records the Pipeline Task that must process the committed bytes.
func (s *Store) CommitFileMutation(
	ctx context.Context,
	mutation *model.FileMutation,
	file *model.File,
	uploadID string,
	task *model.Task,
	versions ...*model.FileVersion,
) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var state string
	if err := tx.QueryRowContext(ctx, `SELECT state FROM file_mutations WHERE id = ?`, mutation.ID).Scan(&state); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if state != model.FileMutationStateFSApplied {
		return errors.New("file mutation is not ready for database commit")
	}

	var occupiedID string
	var occupiedIsDir bool
	targetErr := tx.QueryRowContext(ctx, `
SELECT id, is_dir
FROM files
WHERE path = ? AND name = ? COLLATE NOCASE AND deleted_at IS NULL
LIMIT 1`, file.Path, file.Name).Scan(&occupiedID, &occupiedIsDir)
	if targetErr != nil && !errors.Is(targetErr, sql.ErrNoRows) {
		return targetErr
	}

	now := time.Now().UTC()
	var version *model.FileVersion
	if len(versions) > 0 {
		version = versions[0]
	}
	switch mutation.Kind {
	case model.FileMutationKindUploadCreate, model.FileMutationKindCopyCreate:
		if targetErr == nil {
			return ErrPathConflict
		}
		file.CreatedAt = now
		file.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `
INSERT INTO files (
    id, name, path, storage_path, size, mime_type, is_dir, parent_id,
    status, chunk_count, created_at, updated_at
)
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
		); err != nil {
			return normalizeFilePathConflict(err)
		}
	case model.FileMutationKindUploadReplace, model.FileMutationKindCopyReplace, model.FileMutationKindVersionRestore:
		if targetErr != nil || occupiedIsDir || occupiedID != mutation.TargetFileID {
			return ErrPathConflict
		}
		result, err := tx.ExecContext(ctx, `
UPDATE files
SET storage_path = ?,
    size = ?,
    mime_type = ?,
    status = ?,
    chunk_count = ?,
    updated_at = ?
WHERE id = ? AND is_dir = 0 AND deleted_at IS NULL`,
			file.StoragePath,
			file.Size,
			file.MimeType,
			file.Status,
			file.ChunkCount,
			now,
			mutation.TargetFileID,
		)
		if err := affected(result, err); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE file_id = ?`, mutation.TargetFileID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM file_metadata WHERE file_id = ?`, mutation.TargetFileID); err != nil {
			return err
		}
		file.CreatedAt = file.CreatedAt.UTC()
		file.UpdatedAt = now
	default:
		return errors.New("unsupported file mutation kind")
	}
	if version != nil {
		if err := tx.QueryRowContext(ctx, `
SELECT COALESCE(MAX(version_no), 0) + 1
FROM file_versions
WHERE file_id = ?`, version.FileID).Scan(&version.VersionNo); err != nil {
			return err
		}
		version.CreatedAt = now
		if _, err := tx.ExecContext(ctx, `
INSERT INTO file_versions (
    id, file_id, version_no, storage_path, size, mime_type, sha256, source, created_at
)
VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
			version.ID,
			version.FileID,
			version.VersionNo,
			version.StoragePath,
			version.Size,
			version.MimeType,
			version.SHA256,
			version.Source,
			version.CreatedAt,
		); err != nil {
			return err
		}
	}

	if uploadID != "" {
		result, err := tx.ExecContext(ctx, `UPDATE upload_sessions SET status = ? WHERE id = ?`, model.UploadStatusDone, uploadID)
		if err := affected(result, err); err != nil {
			return err
		}
	}
	if task != nil {
		task.CreatedAt = now
		task.UpdatedAt = now
		if _, err := tx.ExecContext(ctx, `
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
		); err != nil {
			return normalizeActiveTaskConflict(err)
		}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE file_mutations
SET state = ?, updated_at = ?
WHERE id = ? AND state = ?`,
		model.FileMutationStateDBCommitted,
		now,
		mutation.ID,
		model.FileMutationStateFSApplied,
	)
	if err := affected(result, err); err != nil {
		return err
	}
	return tx.Commit()
}

type mutationScanner interface {
	Scan(dest ...any) error
}

func scanFileMutation(scanner mutationScanner) (model.FileMutation, error) {
	var mutation model.FileMutation
	var targetFileID sql.NullString
	var oldStoragePath sql.NullString
	var finalStoragePath sql.NullString
	var errorText sql.NullString
	err := scanner.Scan(
		&mutation.ID,
		&mutation.Kind,
		&mutation.State,
		&mutation.VirtualPath,
		&targetFileID,
		&mutation.StagedPath,
		&oldStoragePath,
		&finalStoragePath,
		&errorText,
		&mutation.CreatedAt,
		&mutation.UpdatedAt,
	)
	if err != nil {
		return mutation, err
	}
	if targetFileID.Valid {
		mutation.TargetFileID = targetFileID.String
	}
	if oldStoragePath.Valid {
		mutation.OldStoragePath = oldStoragePath.String
	}
	if finalStoragePath.Valid {
		mutation.FinalStoragePath = finalStoragePath.String
	}
	if errorText.Valid {
		mutation.Error = errorText.String
	}
	return mutation, nil
}
