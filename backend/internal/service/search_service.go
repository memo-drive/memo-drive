package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

var (
	ErrEmptyQuery         = errors.New("query is required")
	ErrServiceUnavailable = errors.New("service is unavailable")
)

const (
	defaultRAGTopK         = 5
	defaultSearchTopK      = 10
	defaultMultiQueryCount = 3
	defaultRRFK            = 60
	maxRAGTopK             = 50
	maxSearchTopK          = 100
	defaultSnippetLength   = 240
	minCandidateTopK       = 20
	defaultFileLimit       = 50
	maxFileLimit           = 200
)

const multiQueryPrompt = `You are the search query expander for the personal cloud drive MemoDrive. The user's files include documents, notes, spreadsheets, presentations, images, and audio transcriptions.

Task: Generate %d supplementary queries for the search query below in order to maximize vector search recall diversity.

Expansion strategies (each variant should focus on a different strategy):
- Synonym substitution: rewrite using synonyms or alternative phrasing
- Granularity adjustment: make it more specific or more general
- Document perspective: express as document or section titles would phrase it
- Cross-lingual: if the original query is in Chinese, generate equivalent English key phrases, and vice versa

Constraints:
- Output one query per line, without numbering, explanation, or repeating the original query
- Each query should be 5-25 characters (or equivalent English words)
- Output query text only

Original query: %s`

// SearchService provides multi-strategy document search: vector similarity (ChromaDB),
// BM25 full-text (SQLite FTS5), and hybrid fusion via Reciprocal Rank Fusion (RRF).
// It supports query expansion, intent parsing, and score-based ranking.
type SearchService struct {
	cfg      *config.Config
	store    *store.Store
	llm      llm.Provider
	vectorDB vectordb.VectorStore
}

// NewSearchService creates a new SearchService.
func NewSearchService(cfg *config.Config, db *store.Store, llmProvider llm.Provider, vectorDB vectordb.VectorStore) *SearchService {
	return &SearchService{
		cfg:      cfg,
		store:    db,
		llm:      llmProvider,
		vectorDB: vectorDB,
	}
}

// Search executes a chunk-level search using the configured retrieval strategies.
func (s *SearchService) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	started := time.Now()
	plan, earlyResponse, err := s.buildChunkRetrievalPlan(ctx, req, started)
	if err != nil {
		return nil, err
	}
	if earlyResponse != nil {
		return earlyResponse, nil
	}
	if !plan.Retrieval.Available() {
		return nil, fmt.Errorf("%w: no chunk retrieval backend is configured", ErrServiceUnavailable)
	}
	log.Printf("level=info component=search event=search_begin query_chars=%d top_k=%d file_filter=%d provider=%s queries=%d vector=%t bm25=%t hybrid=%t",
		len([]rune(plan.Query)), plan.TopK, len(plan.FileIDs), s.searchProviderName(), len(plan.Queries), plan.Retrieval.Vector, plan.Retrieval.BM25, s.hybridSearch())

	sources, err := s.retrieveChunkEvidence(ctx, plan)
	if err != nil {
		log.Printf("level=error component=search event=search_failed query_chars=%d duration_ms=%d err=%q", len([]rune(plan.Query)), time.Since(started).Milliseconds(), err)
		return nil, err
	}
	if sources == nil {
		sources = []SourceChunk{}
	}
	logScoreDistribution("search", sources)
	log.Printf("level=info component=search event=search_complete results=%d queries=%d duration_ms=%d",
		len(sources), len(plan.Queries), time.Since(started).Milliseconds())

	return &SearchResponse{
		Query:   plan.Query,
		Results: sources,
		Intent:  plan.Intent,
	}, nil
}

func (s *SearchService) searchProviderName() string {
	if s == nil || s.llm == nil {
		return "none"
	}
	return s.llm.Name()
}
