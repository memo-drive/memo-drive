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
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

type handlerMockProvider struct{}

func (p *handlerMockProvider) Name() string {
	return "mock"
}

func (p *handlerMockProvider) Chat(ctx context.Context, messages []llm.Message) (<-chan string, error) {
	ch := make(chan string, 2)
	ch <- "hello"
	ch <- " world"
	close(ch)
	return ch, nil
}

func (p *handlerMockProvider) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	return "", nil
}

func (p *handlerMockProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	return [][]float32{{0.1, 0.2}}, nil
}

type handlerVectorStore struct{}

func (s *handlerVectorStore) EnsureCollection(ctx context.Context, name string) error {
	return nil
}

func (s *handlerVectorStore) Upsert(ctx context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error {
	return nil
}

func (s *handlerVectorStore) Query(ctx context.Context, collection string, queryEmbedding []float32, nResults int) (*vectordb.QueryResult, error) {
	return &vectordb.QueryResult{
		IDs:       []string{"file-a#0"},
		Documents: []string{"source content"},
		Distances: []float32{0.2},
		Metadatas: []map[string]any{
			{"file_id": "file-a", "file_name": "Guide.md", "heading": "Intro", "chunk_index": 0},
		},
	}, nil
}

func (s *handlerVectorStore) Delete(ctx context.Context, collection string, ids []string) error {
	return nil
}

func TestAIHandlerSearchReturnsResults(t *testing.T) {
	app := newAIHandlerTestApp(t)
	body := bytes.NewBufferString(`{"query":"source","top_k":1}`)
	req := httptest.NewRequest(http.MethodPost, "/ai/search", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, payload)
	}
	var response service.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Results) != 1 || response.Results[0].FileName != "Guide.md" {
		t.Fatalf("unexpected search response: %#v", response)
	}
}

func TestAIHandlerChatStreamsSourcesThenDelta(t *testing.T) {
	app := newAIHandlerTestApp(t)
	body := bytes.NewBufferString(`{"prompt":"answer from files","top_k":1}`)
	req := httptest.NewRequest(http.MethodPost, "/ai/chat", body)
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, payload)
	}
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	text := string(payload)
	conversationIndex := strings.Index(text, "event: conversation")
	sourceIndex := strings.Index(text, "event: sources")
	deltaIndex := strings.Index(text, `"delta":"hello"`)
	doneIndex := strings.Index(text, "event: done")
	if conversationIndex < 0 || sourceIndex < 0 || deltaIndex < 0 || doneIndex < 0 {
		t.Fatalf("expected conversation, sources, delta and done events, got:\n%s", text)
	}
	if !(conversationIndex < sourceIndex && sourceIndex < deltaIndex && deltaIndex < doneIndex) {
		t.Fatalf("unexpected event order:\n%s", text)
	}
}

func newAIHandlerTestApp(t *testing.T) *fiber.App {
	t.Helper()
	cfg := &config.Config{RAG: config.RAGConfig{TopK: 2, SearchTopK: 2, MaxContextChars: 4000}}
	provider := &handlerMockProvider{}
	vector := &handlerVectorStore{}
	search := service.NewSearchService(cfg, nil, provider, vector)
	rag := service.NewRAGService(cfg, provider, search)
	db := newHandlerTestStore(t)
	convs := service.NewConversationService(db)
	app := fiber.New()
	NewAIHandler(provider, rag, search, convs).Register(app)
	return app
}

func newHandlerTestStore(t *testing.T) *store.Store {
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
