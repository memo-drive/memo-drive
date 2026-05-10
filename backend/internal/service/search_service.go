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

const multiQueryPrompt = `你是一个搜索查询扩展助手。给定一个搜索问题，生成 %d 个不同角度的等价查询。
每行一个查询，不编号，不解释。

原始问题: %s`

type SearchService struct {
	cfg      *config.Config
	store    *store.Store
	llm      llm.Provider
	vectorDB vectordb.VectorStore
}

func NewSearchService(cfg *config.Config, db *store.Store, llmProvider llm.Provider, vectorDB vectordb.VectorStore) *SearchService {
	return &SearchService{
		cfg:      cfg,
		store:    db,
		llm:      llmProvider,
		vectorDB: vectorDB,
	}
}

func (s *SearchService) Search(ctx context.Context, req SearchRequest) (*SearchResponse, error) {
	started := time.Now()
	if s == nil || s.llm == nil {
		return nil, fmt.Errorf("%w: llm provider is not configured", ErrServiceUnavailable)
	}
	if s.vectorDB == nil {
		return nil, fmt.Errorf("%w: vector store is not configured", ErrServiceUnavailable)
	}

	plan, earlyResponse, err := s.buildChunkRetrievalPlan(ctx, req, started)
	if err != nil {
		return nil, err
	}
	if earlyResponse != nil {
		return earlyResponse, nil
	}
	log.Printf("level=info component=search event=search_begin query_chars=%d top_k=%d file_filter=%d provider=%s queries=%d hybrid=%t",
		len([]rune(plan.Query)), plan.TopK, len(plan.FileIDs), s.llm.Name(), len(plan.Queries), s.hybridSearch())

	sources, err := s.retrieveChunkEvidence(ctx, plan)
	if err != nil {
		log.Printf("level=error component=search event=search_failed query_chars=%d duration_ms=%d err=%q", len([]rune(plan.Query)), time.Since(started).Milliseconds(), err)
		return nil, err
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
