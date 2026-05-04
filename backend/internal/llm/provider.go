package llm

import (
	"context"
	"log"
	"strings"

	"github.com/memodrive/backend/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Provider interface {
	Name() string
	Chat(ctx context.Context, messages []Message) (<-chan string, error)
	Complete(ctx context.Context, messages []Message) (string, error)
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

func NewProvider(cfg config.LLMConfig) Provider {
	if strings.TrimSpace(cfg.OpenAIAPIKey) != "" {
		log.Printf("level=info component=llm event=provider_selected provider=openai base_url=%s chat_model=%q embed_model=%q", cfg.OpenAIBaseURL, cfg.OpenAIChat, cfg.OpenAIEmbed)
		return NewOpenAI(cfg.OpenAIBaseURL, cfg.OpenAIAPIKey, cfg.OpenAIChat, cfg.OpenAIEmbed)
	}
	log.Printf("level=info component=llm event=provider_selected provider=ollama reason=no_openai_api_key base_url=%s chat_model=%q embed_model=%q", cfg.OllamaBaseURL, cfg.OllamaChat, cfg.OllamaEmbed)
	return NewOllama(cfg.OllamaBaseURL, cfg.OllamaChat, cfg.OllamaEmbed)
}
