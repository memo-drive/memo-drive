package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOllamaEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "embed-model" {
			t.Fatalf("unexpected model: %s", request.Model)
		}
		if len(request.Input) != 2 {
			t.Fatalf("expected 2 inputs, got %d", len(request.Input))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"embeddings": [][]float32{{0.1, 0.2}, {0.3, 0.4}},
		})
	}))
	defer server.Close()

	provider := NewOllama(server.URL, "chat-model", "embed-model")
	embeddings, err := provider.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(embeddings) != 2 || len(embeddings[0]) != 2 || embeddings[1][1] != 0.4 {
		t.Fatalf("unexpected embeddings: %#v", embeddings)
	}
}

func TestOllamaChatStreamsNDJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var request struct {
			Model    string    `json:"model"`
			Messages []Message `json:"messages"`
			Stream   bool      `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "chat-model" || !request.Stream {
			t.Fatalf("unexpected request: %#v", request)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		_, _ = w.Write([]byte(`{"message":{"content":"你"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"message":{"content":"好"},"done":false}` + "\n"))
		_, _ = w.Write([]byte(`{"done":true}` + "\n"))
	}))
	defer server.Close()

	provider := NewOllama(server.URL, "chat-model", "embed-model")
	stream, err := provider.Chat(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
	var builder strings.Builder
	for chunk := range stream {
		builder.WriteString(chunk)
	}
	if builder.String() != "你好" {
		t.Fatalf("unexpected stream content: %q", builder.String())
	}
}

func TestOllamaComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var request struct {
			Model    string    `json:"model"`
			Messages []Message `json:"messages"`
			Stream   bool      `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "chat-model" || request.Stream {
			t.Fatalf("unexpected request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message": map[string]string{"content": "expanded query"},
		})
	}))
	defer server.Close()

	provider := NewOllama(server.URL, "chat-model", "embed-model")
	result, err := provider.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if result != "expanded query" {
		t.Fatalf("unexpected completion: %q", result)
	}
}
