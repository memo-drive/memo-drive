// Package llm provides a unified interface for language model operations
// (chat, completion, embedding) with support for Ollama and OpenAI backends.
package llm

import (
	"context"
	"log"
	"strings"

	"github.com/memodrive/backend/internal/config"
)

// Message represents a single turn in a chat conversation.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider is the unified LLM interface. Implementations handle chat streaming,
// non-streaming completion, and text embedding.
type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message) (<-chan string, error)
	Complete(ctx context.Context, messages []Message) (string, error)
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// NewProvider creates the appropriate LLM provider based on configuration.
// If an OpenAI API key is set, OpenAI is used; otherwise Ollama is the default.
func NewProvider(cfg config.LLMConfig) Provider {
	if strings.TrimSpace(cfg.OpenAIAPIKey) != "" {
		log.Printf("level=info component=llm event=provider_selected provider=openai base_url=%s chat_model=%q embed_model=%q", cfg.OpenAIBaseURL, cfg.OpenAIChat, cfg.OpenAIEmbed)
		return NewOpenAI(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.OpenAIChat, cfg.OpenAIEmbed)
	}
	log.Printf("level=info component=llm event=provider_selected provider=ollama reason=no_openai_api_key base_url=%s chat_model=%q embed_model=%q", cfg.OllamaBaseURL, cfg.OllamaChat, cfg.OllamaEmbed)
	return NewOllama(cfg.OllamaBaseURL, cfg.OllamaChat, cfg.OllamaEmbed)
}
