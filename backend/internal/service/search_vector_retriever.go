package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/memodrive/backend/internal/vectordb"
)

type vectorQueryEmbedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

type vectorQueryStore interface {
	Query(ctx context.Context, collection string, queryEmbedding []float32, nResults int) (*vectordb.QueryResult, error)
}

type vectorChunkRetriever struct {
	Embedder   vectorQueryEmbedder
	Store      vectorQueryStore
	Collection string
	Mapper     vectorChunkMapper
}

type vectorChunkRetrievalOptions struct {
	Query         string
	FileIDs       []string
	CandidateTopK int
}

type vectorChunkRetrievalResult struct {
	Sources    []SourceChunk
	Candidates int
	Dimensions int
}

func (r vectorChunkRetriever) Retrieve(ctx context.Context, opts vectorChunkRetrievalOptions) (vectorChunkRetrievalResult, error) {
	started := time.Now()
	if r.Embedder == nil {
		return vectorChunkRetrievalResult{}, fmt.Errorf("%w: llm provider is not configured", ErrServiceUnavailable)
	}
	if r.Store == nil {
		return vectorChunkRetrievalResult{}, fmt.Errorf("%w: vector store is not configured", ErrServiceUnavailable)
	}
	candidateTopK := opts.CandidateTopK
	if candidateTopK <= 0 {
		candidateTopK = defaultSearchTopK
	}

	embedStarted := time.Now()
	embeddings, err := r.Embedder.Embed(ctx, []string{opts.Query})
	if err != nil {
		log.Printf("level=error component=search event=search_single_failed stage=embed query_chars=%d duration_ms=%d err=%q", len([]rune(opts.Query)), time.Since(started).Milliseconds(), err)
		return vectorChunkRetrievalResult{}, fmt.Errorf("embed search query: %w", err)
	}
	if len(embeddings) != 1 || len(embeddings[0]) == 0 {
		err := fmt.Errorf("search embedding count mismatch: got %d", len(embeddings))
		log.Printf("level=error component=search event=search_single_failed stage=embed query_chars=%d duration_ms=%d err=%q", len([]rune(opts.Query)), time.Since(started).Milliseconds(), err)
		return vectorChunkRetrievalResult{}, err
	}
	log.Printf("level=info component=search event=embed_complete dimensions=%d duration_ms=%d", len(embeddings[0]), time.Since(embedStarted).Milliseconds())

	collection := r.Collection
	if collection == "" {
		collection = vectordb.DefaultCollection
	}
	result, err := r.Store.Query(ctx, collection, embeddings[0], candidateTopK)
	if err != nil {
		log.Printf("level=error component=search event=search_single_failed stage=vector_query query_chars=%d duration_ms=%d err=%q", len([]rune(opts.Query)), time.Since(started).Milliseconds(), err)
		return vectorChunkRetrievalResult{}, fmt.Errorf("query vector store: %w", err)
	}
	return vectorChunkRetrievalResult{
		Sources: r.Mapper.Map(result, vectorChunkMappingOptions{
			FileIDs: opts.FileIDs,
			TopK:    candidateTopK,
		}),
		Candidates: queryResultLen(result),
		Dimensions: len(embeddings[0]),
	}, nil
}

func (s *SearchService) vectorChunkRetriever() vectorChunkRetriever {
	return vectorChunkRetriever{
		Embedder:   s.llm,
		Store:      s.vectorDB,
		Collection: vectordb.DefaultCollection,
		Mapper:     s.vectorChunkMapper(),
	}
}
