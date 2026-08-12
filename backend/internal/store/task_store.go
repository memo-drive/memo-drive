package store

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/model"
)

var ErrTaskAlreadyActive = errors.New("task already active")

type TaskListFilter struct {
	Status string
	FileID string
	Cursor string
	Limit  int
}

type taskPageCursor struct {
	Version   int       `json:"v"`
	Status    string    `json:"status,omitempty"`
	FileID    string    `json:"file_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

const (
	defaultTaskPageLimit = 50
	maxTaskPageLimit     = 100
)

// CreateRetryTask atomically reserves one active pipeline slot for a File.
func (s *Store) CreateRetryTask(ctx context.Context, task *model.Task) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var exists int
	err = tx.QueryRowContext(ctx, `
SELECT 1
FROM tasks
WHERE file_id = ? AND status IN (?, ?)
LIMIT 1`, task.FileID, model.TaskStatusPending, model.TaskStatusProcessing).Scan(&exists)
	if err == nil {
		return ErrTaskAlreadyActive
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	now := time.Now().UTC()
	task.CreatedAt = now
	task.UpdatedAt = now
	if _, err := tx.ExecContext(ctx, `
INSERT INTO tasks (id, file_id, type, status, progress, error, retry_count, retry_of_task_id, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?)`,
		task.ID,
		task.FileID,
		task.Type,
		task.Status,
		task.Progress,
		nullableString(task.Error),
		task.RetryCount,
		task.RetryOfTaskID,
		task.CreatedAt,
		task.UpdatedAt,
	); err != nil {
		return normalizeActiveTaskConflict(err)
	}
	return tx.Commit()
}

func normalizeActiveTaskConflict(err error) error {
	if err == nil {
		return nil
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint failed: tasks.file_id") {
		return ErrTaskAlreadyActive
	}
	return err
}

// ListTaskItems returns the most recently created pipeline Tasks with their
// current File summary.
func (s *Store) ListTaskItems(ctx context.Context, filter TaskListFilter) ([]model.TaskListItem, string, bool, error) {
	limit := normalizeTaskPageLimit(filter.Limit)
	clauses := []string{"1 = 1"}
	args := make([]any, 0, 6)
	if filter.Status != "" {
		clauses = append(clauses, "t.status = ?")
		args = append(args, filter.Status)
	}
	if filter.FileID != "" {
		clauses = append(clauses, "t.file_id = ?")
		args = append(args, filter.FileID)
	}
	if filter.Cursor != "" {
		cursor, err := decodeTaskPageCursor(filter.Cursor, filter.Status, filter.FileID)
		if err != nil {
			return nil, "", false, err
		}
		clauses = append(clauses, "(t.created_at < ? OR (t.created_at = ? AND t.id < ?))")
		args = append(args, cursor.CreatedAt, cursor.CreatedAt, cursor.ID)
	}
	args = append(args, limit+1)
	rows, err := s.db.QueryContext(ctx, `
SELECT t.id, t.file_id, t.type, t.status, t.progress, t.error, t.retry_count, t.retry_of_task_id,
       t.created_at, t.updated_at,
       f.id, f.name, f.path, f.size, f.mime_type, f.status
FROM tasks t
JOIN files f ON f.id = t.file_id
WHERE `+strings.Join(clauses, " AND ")+`
ORDER BY t.created_at DESC, t.id DESC
LIMIT ?`, args...)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()

	items := make([]model.TaskListItem, 0)
	for rows.Next() {
		var item model.TaskListItem
		var errText sql.NullString
		var retryOfTaskID sql.NullString
		if err := rows.Scan(
			&item.ID,
			&item.FileID,
			&item.Type,
			&item.Status,
			&item.Progress,
			&errText,
			&item.RetryCount,
			&retryOfTaskID,
			&item.CreatedAt,
			&item.UpdatedAt,
			&item.File.ID,
			&item.File.Name,
			&item.File.Path,
			&item.File.Size,
			&item.File.MimeType,
			&item.File.Status,
		); err != nil {
			return nil, "", false, err
		}
		if errText.Valid {
			item.Error = &errText.String
		}
		if retryOfTaskID.Valid {
			item.RetryOfTaskID = retryOfTaskID.String
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", false, err
	}
	hasMore := len(items) > limit
	nextCursor := ""
	if hasMore {
		items = items[:limit]
		encoded, err := encodeTaskPageCursor(filter.Status, filter.FileID, items[len(items)-1])
		if err != nil {
			return nil, "", false, err
		}
		nextCursor = encoded
	}
	return items, nextCursor, hasMore, nil
}

func normalizeTaskPageLimit(limit int) int {
	if limit <= 0 {
		return defaultTaskPageLimit
	}
	if limit > maxTaskPageLimit {
		return maxTaskPageLimit
	}
	return limit
}

func encodeTaskPageCursor(status, fileID string, item model.TaskListItem) (string, error) {
	payload, err := json.Marshal(taskPageCursor{
		Version:   1,
		Status:    status,
		FileID:    fileID,
		CreatedAt: item.CreatedAt,
		ID:        item.ID,
	})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeTaskPageCursor(raw, status, fileID string) (*taskPageCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}
	var cursor taskPageCursor
	if err := json.Unmarshal(payload, &cursor); err != nil || cursor.Version != 1 ||
		cursor.ID == "" || cursor.CreatedAt.IsZero() || cursor.Status != status || cursor.FileID != fileID {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}
