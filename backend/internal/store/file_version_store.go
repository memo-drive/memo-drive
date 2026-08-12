package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/memodrive/backend/internal/model"
)

func (s *Store) ListFileVersions(ctx context.Context, fileID string) ([]model.FileVersion, error) {
	if _, err := s.GetFileIncludeDeleted(ctx, fileID); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, file_id, version_no, storage_path, size, mime_type, sha256, source, created_at
FROM file_versions
WHERE file_id = ?
ORDER BY version_no DESC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]model.FileVersion, 0)
	for rows.Next() {
		version, err := scanFileVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

func (s *Store) GetFileVersion(ctx context.Context, fileID, versionID string) (*model.FileVersion, error) {
	version, err := scanFileVersion(s.db.QueryRowContext(ctx, `
SELECT id, file_id, version_no, storage_path, size, mime_type, sha256, source, created_at
FROM file_versions
WHERE id = ? AND file_id = ?`, versionID, fileID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func (s *Store) DeleteFileVersion(ctx context.Context, fileID, versionID string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM file_versions WHERE id = ? AND file_id = ?`, versionID, fileID)
	return affected(result, err)
}

func (s *Store) TotalFileVersionSize(ctx context.Context) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(size), 0) FROM file_versions`).Scan(&total)
	return total, err
}

func (s *Store) ListFileVersionsForRetention(ctx context.Context, maxCount int, olderThan time.Time, limit int) ([]model.FileVersion, error) {
	if maxCount <= 0 || limit <= 0 {
		return []model.FileVersion{}, nil
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, file_id, version_no, storage_path, size, mime_type, sha256, source, created_at
FROM (
    SELECT id, file_id, version_no, storage_path, size, mime_type, sha256, source, created_at,
           ROW_NUMBER() OVER (PARTITION BY file_id ORDER BY version_no DESC) AS version_rank
    FROM file_versions
)
WHERE version_rank > ? OR (version_rank > 1 AND created_at < ?)
ORDER BY created_at ASC, id ASC
LIMIT ?`, maxCount, olderThan.UTC(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	versions := make([]model.FileVersion, 0)
	for rows.Next() {
		version, err := scanFileVersion(rows)
		if err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

type fileVersionScanner interface {
	Scan(dest ...any) error
}

func scanFileVersion(scanner fileVersionScanner) (model.FileVersion, error) {
	var version model.FileVersion
	var mimeType sql.NullString
	var sha256 sql.NullString
	err := scanner.Scan(
		&version.ID,
		&version.FileID,
		&version.VersionNo,
		&version.StoragePath,
		&version.Size,
		&mimeType,
		&sha256,
		&version.Source,
		&version.CreatedAt,
	)
	if err != nil {
		return model.FileVersion{}, err
	}
	version.MimeType = mimeType.String
	version.SHA256 = sha256.String
	if version.SHA256 == "" {
		version.ChecksumStatus = "missing"
	} else {
		version.ChecksumStatus = "recorded"
	}
	return version, nil
}
