package llm

import (
	"testing"

	"github.com/memodrive/backend/internal/config"
)

func TestNewProviderPrefersOpenAIWhenAPIKeyConfigured(t *testing.T) {
	provider := NewProvider(config.LLMConfig{
		OpenAIBaseURL: "https://example.test/v1",
		OpenAIAPIKey:  "test-key",
		OpenAIChat:    "chat-model",
		OpenAIEmbed:   "embed-model",
		OllamaBaseURL: "http://ollama:11434",
	})
	if provider.Name() != "openai" {
		t.Fatalf("expected openai provider, got %s", provider.Name())
	}
	openai, ok := provider.(*OpenAIProvider)
	if !ok {
		t.Fatalf("expected *OpenAIProvider, got %T", provider)
	}
	if openai.BaseURL != "https://example.test/v1" {
		t.Fatalf("unexpected base url: %s", openai.BaseURL)
	}
}

func TestNewProviderFallsBackToOllamaWithoutAPIKey(t *testing.T) {
	provider := NewProvider(config.LLMConfig{
		OllamaBaseURL: "http://ollama.test",
		OllamaChat:    "chat-model",
		OllamaEmbed:   "embed-model",
	})
	if provider.Name() != "ollama" {
		t.Fatalf("expected ollama provider, got %s", provider.Name())
	}
	ollama, ok := provider.(*OllamaProvider)
	if !ok {
		t.Fatalf("expected *OllamaProvider, got %T", provider)
	}
	if ollama.BaseURL != "http://ollama.test" {
		t.Fatalf("unexpected base url: %s", ollama.BaseURL)
	}
}
