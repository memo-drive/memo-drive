package store

import (
	"context"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/model"
)

func TestConversationStoreCreateGetListAndTouch(t *testing.T) {
	db := newSearchTestStore(t)
	ctx := context.Background()

	oldConv := &model.Conversation{
		ID:      "conv-old",
		Title:   "Old",
		Mode:    "rag",
		FileIDs: []string{"file-a"},
	}
	newConv := &model.Conversation{
		ID:    "conv-new",
		Title: "New",
		Mode:  "search",
	}
	if err := db.CreateConversation(ctx, oldConv); err != nil {
		t.Fatalf("create old conversation: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	if err := db.CreateConversation(ctx, newConv); err != nil {
		t.Fatalf("create new conversation: %v", err)
	}

	got, err := db.GetConversation(ctx, oldConv.ID)
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if got.Mode != "rag" || len(got.FileIDs) != 1 || got.FileIDs[0] != "file-a" {
		t.Fatalf("unexpected conversation: %#v", got)
	}

	items, err := db.ListConversations(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list conversations: %v", err)
	}
	if len(items) != 2 || items[0].ID != newConv.ID {
		t.Fatalf("expected newest conversation first, got %#v", items)
	}

	time.Sleep(2 * time.Millisecond)
	if err := db.TouchConversation(ctx, oldConv.ID); err != nil {
		t.Fatalf("touch conversation: %v", err)
	}
	items, err = db.ListConversations(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list after touch: %v", err)
	}
	if len(items) != 2 || items[0].ID != oldConv.ID {
		t.Fatalf("expected touched conversation first, got %#v", items)
	}
}

func TestConversationStoreAppendListAndDeleteCascade(t *testing.T) {
	db := newSearchTestStore(t)
	ctx := context.Background()
	conv := &model.Conversation{ID: "conv", Title: "Chat", Mode: "rag"}
	if err := db.CreateConversation(ctx, conv); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	messages := []*model.Message{
		{ID: "msg-user", ConversationID: conv.ID, Role: "user", Content: "hello"},
		{
			ID:             "msg-assistant",
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        "world",
			Sources: []model.SourceChunk{{
				ID:       "file-a#0",
				FileID:   "file-a",
				FileName: "Guide.md",
				Score:    0.8,
			}},
		},
	}
	for _, msg := range messages {
		if err := db.AppendMessage(ctx, msg); err != nil {
			t.Fatalf("append message %s: %v", msg.ID, err)
		}
		time.Sleep(time.Millisecond)
	}

	got, err := db.ListMessages(ctx, conv.ID, 10)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(got) != 2 || got[0].ID != "msg-user" || got[1].ID != "msg-assistant" {
		t.Fatalf("expected messages in creation order, got %#v", got)
	}
	if len(got[1].Sources) != 1 || got[1].Sources[0].FileName != "Guide.md" {
		t.Fatalf("expected sources round trip, got %#v", got[1].Sources)
	}

	if err := db.DeleteConversation(ctx, conv.ID); err != nil {
		t.Fatalf("delete conversation: %v", err)
	}
	got, err = db.ListMessages(ctx, conv.ID, 10)
	if err != nil {
		t.Fatalf("list messages after delete: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected cascade delete, got %#v", got)
	}
}
