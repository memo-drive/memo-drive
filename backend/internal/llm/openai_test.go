package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header")
		}
		var request struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if request.Model != "embed-model" || len(request.Input) != 2 {
			t.Fatalf("unexpected request: %#v", request)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"index": 1, "embedding": []float32{0.3, 0.4}},
				{"index": 0, "embedding": []float32{0.1, 0.2}},
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "test-key", "chat-model", "embed-model")
	embeddings, err := provider.Embed(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(embeddings) != 2 || embeddings[0][0] != 0.1 || embeddings[1][1] != 0.4 {
		t.Fatalf("unexpected embeddings: %#v", embeddings)
	}
}

func TestOpenAIChatStreamsSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Fatalf("missing authorization header")
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
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"你"}}]}` + "\n\n"))
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"好"}}]}` + "\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "test-key", "chat-model", "embed-model")
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

func TestOpenAIComplete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
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
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "condensed query"}},
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "test-key", "chat-model", "embed-model")
	result, err := provider.Complete(context.Background(), []Message{{Role: "user", Content: "hi"}})
	if err != nil {
		t.Fatalf("complete failed: %v", err)
	}
	if result != "condensed query" {
		t.Fatalf("unexpected completion: %q", result)
	}
}

func TestOpenAIHTTPErrorUsesProviderMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{
				"message": "slow down",
				"type":    "rate_limit",
			},
		})
	}))
	defer server.Close()

	provider := NewOpenAI(server.URL, "test-key", "chat-model", "embed-model")
	_, err := provider.Embed(context.Background(), []string{"hello"})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "rate limited") || !strings.Contains(err.Error(), "slow down") {
		t.Fatalf("unexpected error: %v", err)
	}
}
