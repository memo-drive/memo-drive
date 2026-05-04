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

const (
	defaultConversationListLimit = 50
	maxConversationListLimit     = 500
	defaultMessageListLimit      = 200
	maxMessageListLimit          = 1000
)

func (s *Store) CreateConversation(ctx context.Context, conv *model.Conversation) error {
	if conv == nil {
		return errors.New("conversation is nil")
	}
	now := time.Now().UTC()
	if conv.CreatedAt.IsZero() {
		conv.CreatedAt = now
	}
	if conv.UpdatedAt.IsZero() {
		conv.UpdatedAt = conv.CreatedAt
	}
	fileIDs, err := json.Marshal(conv.FileIDs)
	if err != nil {
		return fmt.Errorf("marshal conversation file_ids: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO conversations(id, title, mode, file_ids, created_at, updated_at)
VALUES(?, ?, ?, ?, ?, ?)`,
		conv.ID,
		strings.TrimSpace(conv.Title),
		conversationModeForDB(conv.Mode),
		string(fileIDs),
		conv.CreatedAt,
		conv.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	return nil
}

func (s *Store) UpdateConversationTitle(ctx context.Context, id, title string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET title = ?, updated_at = ?
WHERE id = ?`,
		strings.TrimSpace(title),
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("update conversation title: %w", err)
	}
	return requireRowsAffected(result)
}

func (s *Store) TouchConversation(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `
UPDATE conversations
SET updated_at = ?
WHERE id = ?`,
		time.Now().UTC(),
		id,
	)
	if err != nil {
		return fmt.Errorf("touch conversation: %w", err)
	}
	return requireRowsAffected(result)
}

func (s *Store) GetConversation(ctx context.Context, id string) (*model.Conversation, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, title, mode, file_ids, created_at, updated_at
FROM conversations
WHERE id = ?`,
		id,
	)
	conv, err := scanConversation(row)
	if err != nil {
		return nil, err
	}
	return conv, nil
}

func (s *Store) ListConversations(ctx context.Context, limit, offset int) ([]model.Conversation, error) {
	limit = clampLimit(limit, defaultConversationListLimit, maxConversationListLimit)
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, title, mode, file_ids, created_at, updated_at
FROM conversations
ORDER BY updated_at DESC, created_at DESC
LIMIT ? OFFSET ?`,
		limit,
		offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]model.Conversation, 0)
	for rows.Next() {
		conv, err := scanConversation(rows)
		if err != nil {
			return nil, err
		}
		conversations = append(conversations, *conv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list conversations rows: %w", err)
	}
	return conversations, nil
}

func (s *Store) DeleteConversation(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM conversations WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete conversation: %w", err)
	}
	return requireRowsAffected(result)
}

func (s *Store) AppendMessage(ctx context.Context, msg *model.Message) error {
	if msg == nil {
		return errors.New("message is nil")
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now().UTC()
	}
	var sources sql.NullString
	if len(msg.Sources) > 0 {
		payload, err := json.Marshal(msg.Sources)
		if err != nil {
			return fmt.Errorf("marshal message sources: %w", err)
		}
		sources = sql.NullString{String: string(payload), Valid: true}
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO messages(id, conversation_id, role, content, sources, created_at)
VALUES(?, ?, ?, ?, ?, ?)`,
		msg.ID,
		msg.ConversationID,
		msg.Role,
		msg.Content,
		sources,
		msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("append message: %w", err)
	}
	return nil
}

func (s *Store) ListMessages(ctx context.Context, convID string, limit int) ([]model.Message, error) {
	limit = clampLimit(limit, defaultMessageListLimit, maxMessageListLimit)
	rows, err := s.db.QueryContext(ctx, `
SELECT id, conversation_id, role, content, sources, created_at
FROM messages
WHERE conversation_id = ?
ORDER BY created_at ASC
LIMIT ?`,
		convID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list messages: %w", err)
	}
	defer rows.Close()

	messages := make([]model.Message, 0)
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, *msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list messages rows: %w", err)
	}
	return messages, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanConversation(row rowScanner) (*model.Conversation, error) {
	var conv model.Conversation
	var title sql.NullString
	var mode sql.NullString
	var fileIDs sql.NullString
	var updatedAt sql.NullTime
	if err := row.Scan(&conv.ID, &title, &mode, &fileIDs, &conv.CreatedAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan conversation: %w", err)
	}
	conv.Title = title.String
	conv.Mode = conversationModeFromDB(mode.String)
	if conv.Mode == "" {
		conv.Mode = "rag"
	}
	conv.UpdatedAt = updatedAt.Time
	if conv.UpdatedAt.IsZero() {
		conv.UpdatedAt = conv.CreatedAt
	}
	if fileIDs.Valid && strings.TrimSpace(fileIDs.String) != "" {
		if err := json.Unmarshal([]byte(fileIDs.String), &conv.FileIDs); err != nil {
			return nil, fmt.Errorf("unmarshal conversation file_ids: %w", err)
		}
	}
	return &conv, nil
}

func scanMessage(row rowScanner) (*model.Message, error) {
	var msg model.Message
	var sources sql.NullString
	if err := row.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &sources, &msg.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan message: %w", err)
	}
	if sources.Valid && strings.TrimSpace(sources.String) != "" {
		if err := json.Unmarshal([]byte(sources.String), &msg.Sources); err != nil {
			return nil, fmt.Errorf("unmarshal message sources: %w", err)
		}
	}
	return &msg, nil
}

func conversationModeForDB(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "search":
		return "search"
	default:
		// The original P0 schema allowed "file_qa" instead of "rag". Writing
		// the legacy value keeps existing databases with that CHECK constraint usable.
		return "file_qa"
	}
}

func conversationModeFromDB(mode string) string {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "search":
		return "search"
	case "rag", "file_qa":
		return "rag"
	default:
		return ""
	}
}

func clampLimit(value, fallback, maximum int) int {
	if value <= 0 {
		value = fallback
	}
	if value > maximum {
		value = maximum
	}
	return value
}

func requireRowsAffected(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}
