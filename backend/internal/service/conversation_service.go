package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

// ConversationService manages AI conversation and message history.
type ConversationService struct {
	store *store.Store
}

// NewConversationService creates a new ConversationService.
func NewConversationService(s *store.Store) *ConversationService {
	return &ConversationService{store: s}
}

func (s *ConversationService) EnsureConversation(ctx context.Context, id, mode, firstUserMsg string, fileIDs []string) (string, error) {
	if s == nil || s.store == nil {
		return "", ErrServiceUnavailable
	}
	if strings.TrimSpace(id) != "" {
		if _, err := s.store.GetConversation(ctx, strings.TrimSpace(id)); err == nil {
			return strings.TrimSpace(id), nil
		}
	}

	title := truncateRunes(strings.TrimSpace(firstUserMsg), 30)
	if title == "" {
		title = "新会话"
	}
	conv := &model.Conversation{
		ID:      uuid.NewString(),
		Title:   title,
		Mode:    normalizeConversationMode(mode),
		FileIDs: cleanFileIDs(fileIDs),
	}
	if err := s.store.CreateConversation(ctx, conv); err != nil {
		return "", err
	}
	return conv.ID, nil
}

func (s *ConversationService) Append(ctx context.Context, msg *model.Message) error {
	if s == nil || s.store == nil {
		return ErrServiceUnavailable
	}
	if msg.ID == "" {
		msg.ID = uuid.NewString()
	}
	msg.Role = strings.TrimSpace(strings.ToLower(msg.Role))
	if msg.Role == "" {
		msg.Role = "user"
	}
	if err := s.store.AppendMessage(ctx, msg); err != nil {
		return err
	}
	return s.store.TouchConversation(ctx, msg.ConversationID)
}

func (s *ConversationService) Rename(ctx context.Context, id, title string) error {
	if s == nil || s.store == nil {
		return ErrServiceUnavailable
	}
	return s.store.UpdateConversationTitle(ctx, strings.TrimSpace(id), strings.TrimSpace(title))
}

func (s *ConversationService) Get(ctx context.Context, id string) (*model.Conversation, []model.Message, error) {
	if s == nil || s.store == nil {
		return nil, nil, ErrServiceUnavailable
	}
	conv, err := s.store.GetConversation(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, nil, err
	}
	messages, err := s.store.ListMessages(ctx, conv.ID, 0)
	if err != nil {
		return nil, nil, err
	}
	return conv, messages, nil
}

func (s *ConversationService) List(ctx context.Context, limit, offset int) ([]model.Conversation, error) {
	if s == nil || s.store == nil {
		return nil, ErrServiceUnavailable
	}
	return s.store.ListConversations(ctx, limit, offset)
}

func (s *ConversationService) Delete(ctx context.Context, id string) error {
	if s == nil || s.store == nil {
		return ErrServiceUnavailable
	}
	return s.store.DeleteConversation(ctx, strings.TrimSpace(id))
}

func normalizeConversationMode(mode string) string {
	if strings.EqualFold(strings.TrimSpace(mode), "search") {
		return "search"
	}
	return "rag"
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func cleanFileIDs(fileIDs []string) []string {
	cleaned := make([]string, 0, len(fileIDs))
	seen := make(map[string]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}
	return cleaned
}
