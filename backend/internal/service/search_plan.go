package service

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/store"
)

type chunkRetrievalPlan struct {
	Query     string
	Intent    *SearchIntent
	FileIDs   []string
	TopK      int
	Queries   []string
	Retrieval chunkRetrievalModes
}

type chunkRetrievalModes struct {
	Vector bool
	BM25   bool
}

func (m chunkRetrievalModes) Available() bool {
	return m.Vector || m.BM25
}

func (s *SearchService) buildChunkRetrievalPlan(ctx context.Context, req SearchRequest, started time.Time) (*chunkRetrievalPlan, *SearchResponse, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, nil, ErrEmptyQuery
	}

	var intent *SearchIntent
	if s.intentParseEnabled() {
		parsed := s.parseIntent(ctx, query)
		intent = &parsed
		if parsed.TextQuery != "" {
			query = parsed.TextQuery
		}
		if parsed.HasFilters() && s.store != nil {
			ids, err := s.fileIDsForIntent(ctx, parsed, store.FileSearchFilter{})
			if err != nil {
				log.Printf("level=warn component=search event=intent_filter_failed err=%q", err)
			} else if len(ids) == 0 {
				log.Printf("level=info component=search event=intent_filter_empty original_chars=%d duration_ms=%d", len([]rune(req.Query)), time.Since(started).Milliseconds())
				return nil, &SearchResponse{Query: effectiveIntentQuery(req.Query, parsed), Results: []SourceChunk{}, Intent: intent}, nil
			} else {
				req.FileIDs = constrainFileIDs(req.FileIDs, ids)
				if len(req.FileIDs) == 0 {
					return nil, &SearchResponse{Query: effectiveIntentQuery(req.Query, parsed), Results: []SourceChunk{}, Intent: intent}, nil
				}
			}
		}
		query = effectiveIntentQuery(req.Query, parsed)
	}

	topK := s.searchTopK(req.TopK)
	return &chunkRetrievalPlan{
		Query:     query,
		Intent:    intent,
		FileIDs:   req.FileIDs,
		TopK:      topK,
		Queries:   s.expandQueries(ctx, query, s.multiQueryCount()),
		Retrieval: s.chunkRetrievalModes(),
	}, nil, nil
}

func (s *SearchService) retrieveChunkEvidence(ctx context.Context, plan *chunkRetrievalPlan) ([]SourceChunk, error) {
	if plan == nil {
		return nil, nil
	}
	return s.searchMulti(ctx, plan.Queries, plan.FileIDs, plan.TopK, plan.Retrieval)
}

func (s *SearchService) chunkRetrievalModes() chunkRetrievalModes {
	return chunkRetrievalModes{
		Vector: s != nil && s.llm != nil && s.vectorDB != nil,
		BM25:   s != nil && s.hybridSearch() && s.store != nil,
	}
}

func candidateLimit(topK int, hasFilter bool) int {
	if topK <= 0 {
		topK = defaultSearchTopK
	}
	if hasFilter {
		return maxInt(minCandidateTopK, topK*4)
	}
	return topK
}
