package service

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

func (s *SearchService) searchSingle(ctx context.Context, query string, fileIDs []string, topK int, modes chunkRetrievalModes) ([]SourceChunk, error) {
	started := time.Now()
	candidateTopK := candidateLimit(topK, len(fileIDs) > 0)
	queryStarted := time.Now()
	candidateCount := 0
	var vectorSources []SourceChunk
	if modes.Vector {
		result, err := s.vectorChunkRetriever().Retrieve(ctx, vectorChunkRetrievalOptions{
			Query:         query,
			FileIDs:       fileIDs,
			CandidateTopK: candidateTopK,
		})
		if err != nil {
			return nil, err
		}
		candidateCount = result.Candidates
		vectorSources = result.Sources
	}

	sources := vectorSources
	usedFusion := false
	if modes.BM25 {
		bm25Started := time.Now()
		bm25Limit := maxInt(candidateTopK, topK*2)
		bm25Result, bm25Err := s.bm25ChunkRetriever().Retrieve(ctx, bm25ChunkRetrievalOptions{
			Query:   query,
			FileIDs: fileIDs,
			Limit:   bm25Limit,
		})
		if bm25Err != nil {
			if modes.Vector {
				log.Printf("level=warn component=search event=bm25_failed query_chars=%d err=%q", len([]rune(query)), bm25Err)
			} else {
				log.Printf("level=error component=search event=search_single_failed stage=bm25 query_chars=%d duration_ms=%d err=%q", len([]rune(query)), time.Since(started).Milliseconds(), bm25Err)
				return nil, fmt.Errorf("search chunk store: %w", bm25Err)
			}
		} else if modes.Vector {
			sources = s.chunkEvidenceFusion().FuseHybrid(vectorSources, bm25Result.Sources)
			usedFusion = true
			log.Printf("level=info component=search event=hybrid_fusion vector=%d bm25=%d fused=%d rrf_k=%d bm25_duration_ms=%d",
				len(vectorSources), len(bm25Result.Sources), len(sources), s.rrfConstant(), time.Since(bm25Started).Milliseconds())
		} else {
			sources = bm25Result.Sources
			log.Printf("level=info component=search event=bm25_complete results=%d duration_ms=%d",
				len(bm25Result.Sources), time.Since(bm25Started).Milliseconds())
		}
	}
	sources = s.rankChunkEvidence(ctx, sources, chunkRankingOptions{
		TopK:            topK,
		NormalizeScores: usedFusion,
	})
	log.Printf("level=info component=search event=query_complete candidates=%d results=%d candidate_top_k=%d query_duration_ms=%d duration_ms=%d",
		candidateCount, len(sources), candidateTopK, time.Since(queryStarted).Milliseconds(), time.Since(started).Milliseconds())
	return sources, nil
}

func (s *SearchService) searchMulti(ctx context.Context, queries []string, fileIDs []string, topK int, modes chunkRetrievalModes) ([]SourceChunk, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if len(queries) == 1 {
		return s.searchSingle(ctx, queries[0], fileIDs, topK, modes)
	}

	results := make([]chunkQueryResult, len(queries))
	var wg sync.WaitGroup
	for i, query := range queries {
		wg.Add(1)
		go func(index int, q string) {
			defer wg.Done()
			sources, err := s.searchSingle(ctx, q, fileIDs, topK, modes)
			results[index] = chunkQueryResult{Sources: sources, Err: err}
		}(i, query)
	}
	wg.Wait()

	merged, err := s.chunkEvidenceFusion().MergeMultiQuery(results, topK)
	if err != nil {
		return nil, err
	}
	log.Printf("level=info component=search event=multi_query_merge queries=%d results=%d errors=%d",
		len(queries), len(merged.Sources), merged.ErrorCount)
	return merged.Sources, nil
}
