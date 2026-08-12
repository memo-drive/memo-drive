package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/memodrive/backend/internal/model"
)

func (s *Store) CreateFileCopyOperation(ctx context.Context, operation *model.FileCopyOperation) error {
	now := time.Now().UTC()
	operation.CreatedAt = now
	operation.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `
INSERT INTO file_copy_operations (id, source_file_id, root_file_id, state, error, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?)`,
		operation.ID,
		operation.SourceID,
		nullableText(operation.RootFileID),
		operation.State,
		nullableText(operation.Error),
		operation.CreatedAt,
		operation.UpdatedAt,
	)
	return err
}

func (s *Store) GetFileCopyOperation(ctx context.Context, id string) (*model.FileCopyOperation, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, source_file_id, root_file_id, state, error, created_at, updated_at
FROM file_copy_operations
WHERE id = ?`, id)
	operation, err := scanFileCopyOperation(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &operation, nil
}

func (s *Store) ListRunningFileCopyOperations(ctx context.Context) ([]model.FileCopyOperation, error) {
	return s.ListFileCopyOperationsByState(ctx, model.FileCopyOperationStateRunning)
}

func (s *Store) ListFileCopyOperationsByState(ctx context.Context, state string) ([]model.FileCopyOperation, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, source_file_id, root_file_id, state, error, created_at, updated_at
FROM file_copy_operations
WHERE state = ?
ORDER BY created_at, id`, state)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := make([]model.FileCopyOperation, 0)
	for rows.Next() {
		operation, err := scanFileCopyOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

// CreateFolderCopyRoot atomically publishes the copied root metadata and binds it to its operation.
func (s *Store) CreateFolderCopyRoot(ctx context.Context, operationID string, file *model.File) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC()
	file.CreatedAt = now
	file.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `
INSERT INTO files (id, name, path, storage_path, size, mime_type, is_dir, parent_id, status, chunk_count, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		file.ID, file.Name, file.Path, file.StoragePath, file.Size, file.MimeType, file.IsDir,
		nullableString(file.ParentID), file.Status, file.ChunkCount, file.CreatedAt, file.UpdatedAt,
	); err != nil {
		return normalizeFilePathConflict(err)
	}
	result, err := tx.ExecContext(ctx, `
UPDATE file_copy_operations
SET root_file_id = ?, updated_at = ?
WHERE id = ? AND state = ? AND root_file_id IS NULL`,
		file.ID, now, operationID, model.FileCopyOperationStateRunning)
	if err := affected(result, err); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateFileCopyOperationState(ctx context.Context, id, state, errorText string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE file_copy_operations
SET state = ?, error = ?, updated_at = ?
WHERE id = ?`, state, nullableText(errorText), time.Now().UTC(), id)
	return affected(result, err)
}

type fileCopyOperationScanner interface {
	Scan(dest ...any) error
}

func scanFileCopyOperation(scanner fileCopyOperationScanner) (model.FileCopyOperation, error) {
	var operation model.FileCopyOperation
	var rootFileID sql.NullString
	var errorText sql.NullString
	err := scanner.Scan(
		&operation.ID,
		&operation.SourceID,
		&rootFileID,
		&operation.State,
		&errorText,
		&operation.CreatedAt,
		&operation.UpdatedAt,
	)
	if rootFileID.Valid {
		operation.RootFileID = rootFileID.String
	}
	if errorText.Valid {
		operation.Error = errorText.String
	}
	return operation, err
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
