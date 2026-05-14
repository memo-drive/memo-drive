package indexing

import (
	"context"
	"errors"
	"fmt"
)

const defaultVectorIndexBatchSize = 20

// Embedder is the interface for generating vector embeddings from text.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// VectorStore is the subset of vectordb.VectorStore needed for indexing.
type VectorStore interface {
	Upsert(ctx context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error
}

// VectorIndexStage runs the embedding and vector store upsert steps for a document.
// It batches texts, calls the embedder (with retries), then upserts to the vector store.
type VectorIndexStage struct {
	Embedder      Embedder
	Store         VectorStore
	Collection    string
	BatchSize     int
	EmbedAttempts int
}

type VectorIndexResult struct {
	Count      int
	Dimensions int
}

// Run executes the full indexing pipeline: embed all texts, then upsert to the vector store.
func (s VectorIndexStage) Run(ctx context.Context, plan DocumentIndexPlan) (VectorIndexResult, error) {
	embeddings, err := s.EmbedTexts(ctx, plan.VectorTexts)
	if err != nil {
		return VectorIndexResult{}, err
	}
	if err := s.UpsertPlan(ctx, plan, embeddings); err != nil {
		return VectorIndexResult{}, err
	}
	return VectorIndexResult{
		Count:      len(embeddings),
		Dimensions: embeddingDimensions(embeddings),
	}, nil
}

// EmbedTexts generates embeddings for the given texts in batches, with retry on failure.
func (s VectorIndexStage) EmbedTexts(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if s.Embedder == nil {
		return nil, errors.New("llm provider is not configured")
	}
	batchSize := s.batchSize()
	attempts := s.embedAttempts()
	totalBatches := (len(texts) + batchSize - 1) / batchSize
	embeddings := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchNo := start/batchSize + 1
		batch := texts[start:end]
		var batchEmbeddings [][]float32
		var lastErr error
		for attempt := 1; attempt <= attempts; attempt++ {
			result, err := s.Embedder.Embed(ctx, batch)
			if err == nil && len(result) != len(batch) {
				err = fmt.Errorf("embedding count mismatch: got %d, want %d", len(result), len(batch))
			}
			if err == nil {
				batchEmbeddings = result
				break
			}
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		}
		if batchEmbeddings == nil {
			return nil, fmt.Errorf("embed batch %d/%d failed: %w", batchNo, totalBatches, lastErr)
		}
		embeddings = append(embeddings, batchEmbeddings...)
	}
	return embeddings, nil
}

// UpsertPlan sends the pre-computed plan entries and generated embeddings to the vector store.
func (s VectorIndexStage) UpsertPlan(ctx context.Context, plan DocumentIndexPlan, embeddings [][]float32) error {
	if len(plan.VectorIDs) == 0 {
		return nil
	}
	if s.Store == nil {
		return errors.New("vector store is not configured")
	}
	return s.Store.Upsert(ctx, s.Collection, plan.VectorIDs, embeddings, plan.VectorTexts, plan.VectorMetadatas)
}

func (s VectorIndexStage) batchSize() int {
	if s.BatchSize <= 0 {
		return defaultVectorIndexBatchSize
	}
	return s.BatchSize
}

func (s VectorIndexStage) embedAttempts() int {
	if s.EmbedAttempts <= 0 {
		return 2
	}
	return s.EmbedAttempts
}

func embeddingDimensions(embeddings [][]float32) int {
	if len(embeddings) == 0 {
		return 0
	}
	return len(embeddings[0])
}
