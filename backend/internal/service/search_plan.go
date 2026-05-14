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

// buildChunkRetrievalPlan constructs the search execution plan from a SearchRequest:
//  1. If intent parsing is enabled, extract structured filters (MIME types, dates, etc.)
//     from the natural language query and rewrite the query to its pure semantic part.
//  2. If intent filters produce an empty file list, return early with empty results.
//  3. Determine retrieval modes:
//       Vector  = available when LLM + ChromaDB are configured
//       BM25    = available when hybrid search is enabled + SQLite has FTS5
//  4. Expand the query into multiple variants for recall diversity.
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

// candidateLimit computes how many candidates to request from each retrieval backend.
// When a file filter is active, we over-request (4x, min 20) because the vector store
// doesn't know about file-level filters — we retrieve extra and apply the filter client-side.
func candidateLimit(topK int, hasFilter bool) int {
	if topK <= 0 {
		topK = defaultSearchTopK
	}
	if hasFilter {
		return maxInt(minCandidateTopK, topK*4)
	}
	return topK
}
