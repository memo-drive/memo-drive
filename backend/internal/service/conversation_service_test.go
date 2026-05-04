package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestConversationServiceLifecycle(t *testing.T) {
	db := newConversationServiceTestStore(t)
	svc := NewConversationService(db)
	ctx := context.Background()

	id, err := svc.EnsureConversation(ctx, "", "rag", "你好世界", nil)
	if err != nil {
		t.Fatalf("ensure conversation: %v", err)
	}
	conv, messages, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("get conversation: %v", err)
	}
	if conv.Title != "你好世界" || conv.Mode != "rag" || len(messages) != 0 {
		t.Fatalf("unexpected new conversation: %#v messages=%#v", conv, messages)
	}

	fallbackID, err := svc.EnsureConversation(ctx, "missing", "rag", "测试", nil)
	if err != nil {
		t.Fatalf("ensure missing conversation: %v", err)
	}
	if fallbackID == "missing" || fallbackID == id {
		t.Fatalf("expected invalid id to create a distinct conversation, got %q", fallbackID)
	}

	if err := svc.Append(ctx, &model.Message{ConversationID: id, Role: "user", Content: "问题"}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := svc.Append(ctx, &model.Message{ConversationID: id, Role: "assistant", Content: "答案"}); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	_, messages, err = svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("get after append: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "user" || messages[1].Role != "assistant" {
		t.Fatalf("unexpected message order: %#v", messages)
	}

	if err := svc.Rename(ctx, id, "新标题"); err != nil {
		t.Fatalf("rename: %v", err)
	}
	list, err := svc.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) == 0 || list[0].Title != "新标题" {
		t.Fatalf("expected renamed conversation in list, got %#v", list)
	}

	if err := svc.Delete(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, messages, err = svc.Get(ctx, id)
	if err == nil || len(messages) != 0 {
		t.Fatalf("expected deleted conversation to be missing, err=%v messages=%#v", err, messages)
	}
}

func newConversationServiceTestStore(t *testing.T) *store.Store {
	t.Helper()
	root := t.TempDir()
	dbPath := filepath.Join(root, "db", "memodrive.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatalf("mkdir db dir: %v", err)
	}
	db, err := store.Open(context.Background(), &config.Config{Storage: config.StorageConfig{DBPath: dbPath}})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
