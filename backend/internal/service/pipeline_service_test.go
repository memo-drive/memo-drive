package service

import (
	"context"
	"errors"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
)

type mockEmbedProvider struct {
	failures int
	batches  [][]string
}

func (p *mockEmbedProvider) Name() string {
	return "mock"
}

func (p *mockEmbedProvider) Chat(ctx context.Context, messages []llm.Message) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (p *mockEmbedProvider) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	return "", nil
}

func (p *mockEmbedProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	p.batches = append(p.batches, append([]string(nil), texts...))
	if p.failures > 0 {
		p.failures--
		return nil, errors.New("temporary failure")
	}
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = []float32{float32(len(texts)), float32(i)}
	}
	return embeddings, nil
}

func TestPipelineBatchEmbedBatchesInputs(t *testing.T) {
	provider := &mockEmbedProvider{}
	service := NewPipelineService(&config.Config{
		Pipeline: config.PipelineConfig{EmbedBatchSize: 2},
	}, nil, provider, nil, nil, nil)

	embeddings, err := service.batchEmbed(context.Background(), []string{"a", "b", "c", "d", "e"})
	if err != nil {
		t.Fatalf("batchEmbed returned error: %v", err)
	}
	if len(embeddings) != 5 {
		t.Fatalf("expected 5 embeddings, got %d", len(embeddings))
	}
	if len(provider.batches) != 3 {
		t.Fatalf("expected 3 batches, got %d", len(provider.batches))
	}
	if got := len(provider.batches[0]); got != 2 {
		t.Fatalf("expected first batch size 2, got %d", got)
	}
	if got := len(provider.batches[2]); got != 1 {
		t.Fatalf("expected last batch size 1, got %d", got)
	}
}

func TestPipelineBatchEmbedRetriesOnce(t *testing.T) {
	provider := &mockEmbedProvider{failures: 1}
	service := NewPipelineService(&config.Config{
		Pipeline: config.PipelineConfig{EmbedBatchSize: 2},
	}, nil, provider, nil, nil, nil)

	embeddings, err := service.batchEmbed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("batchEmbed returned error after retry: %v", err)
	}
	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if len(provider.batches) != 2 {
		t.Fatalf("expected failed call plus retry, got %d calls", len(provider.batches))
	}
}
