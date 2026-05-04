package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func TestConversationHandlerLifecycle(t *testing.T) {
	db := newConversationHandlerStore(t)
	svc := service.NewConversationService(db)
	convID, err := svc.EnsureConversation(context.Background(), "", "rag", "hello history", nil)
	if err != nil {
		t.Fatalf("ensure conversation: %v", err)
	}
	if err := svc.Append(context.Background(), &model.Message{ConversationID: convID, Role: "user", Content: "hello history"}); err != nil {
		t.Fatalf("append message: %v", err)
	}

	app := fiber.New()
	NewConversationHandler(svc).Register(app)

	req := httptest.NewRequest(http.MethodGet, "/conversations", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("list request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected list 200, got %d: %s", resp.StatusCode, payload)
	}
	var listResponse struct {
		Conversations []model.Conversation `json:"conversations"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listResponse); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listResponse.Conversations) != 1 || listResponse.Conversations[0].ID != convID {
		t.Fatalf("unexpected list response: %#v", listResponse)
	}

	renameBody := bytes.NewBufferString(`{"title":"renamed"}`)
	req = httptest.NewRequest(http.MethodPatch, "/conversations/"+convID, renameBody)
	req.Header.Set("Content-Type", "application/json")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("rename request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected rename 204, got %d: %s", resp.StatusCode, payload)
	}

	req = httptest.NewRequest(http.MethodGet, "/conversations/"+convID, nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	var getResponse struct {
		Conversation model.Conversation `json:"conversation"`
		Messages     []model.Message    `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&getResponse); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if getResponse.Conversation.Title != "renamed" || len(getResponse.Messages) != 1 {
		t.Fatalf("unexpected get response: %#v", getResponse)
	}

	req = httptest.NewRequest(http.MethodDelete, "/conversations/"+convID, nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("delete request: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected delete 204, got %d: %s", resp.StatusCode, payload)
	}
}

func newConversationHandlerStore(t *testing.T) *store.Store {
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
