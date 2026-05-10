package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

func (s *SearchService) searchSingle(ctx context.Context, query string, fileIDs []string, topK int) ([]SourceChunk, error) {
	started := time.Now()
	candidateTopK := candidateLimit(topK, len(fileIDs) > 0)
	embedStarted := time.Now()
	embeddings, err := s.llm.Embed(ctx, []string{query})
	if err != nil {
		log.Printf("level=error component=search event=search_single_failed stage=embed query_chars=%d duration_ms=%d err=%q", len([]rune(query)), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("embed search query: %w", err)
	}
	if len(embeddings) != 1 || len(embeddings[0]) == 0 {
		err := fmt.Errorf("search embedding count mismatch: got %d", len(embeddings))
		log.Printf("level=error component=search event=search_single_failed stage=embed query_chars=%d duration_ms=%d err=%q", len([]rune(query)), time.Since(started).Milliseconds(), err)
		return nil, err
	}
	log.Printf("level=info component=search event=embed_complete dimensions=%d duration_ms=%d", len(embeddings[0]), time.Since(embedStarted).Milliseconds())

	queryStarted := time.Now()
	result, err := s.vectorDB.Query(ctx, vectordb.DefaultCollection, embeddings[0], candidateTopK)
	if err != nil {
		log.Printf("level=error component=search event=search_single_failed stage=vector_query query_chars=%d duration_ms=%d err=%q", len([]rune(query)), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("query vector store: %w", err)
	}
	vectorSources := s.mapResults(result, fileIDs, candidateTopK)

	var sources []SourceChunk
	if s.hybridSearch() && s.store != nil {
		bm25Started := time.Now()
		bm25Limit := maxInt(candidateTopK, topK*2)
		bm25Sources, bm25Err := s.store.SearchChunksBM25(ctx, query, fileIDs, bm25Limit)
		if bm25Err != nil {
			log.Printf("level=warn component=search event=bm25_failed query_chars=%d err=%q", len([]rune(query)), bm25Err)
			sources = vectorSources
		} else {
			sources = rrfFusion(vectorSources, bm25Sources, s.rrfConstant())
			log.Printf("level=info component=search event=hybrid_fusion vector=%d bm25=%d fused=%d rrf_k=%d bm25_duration_ms=%d",
				len(vectorSources), len(bm25Sources), len(sources), s.rrfConstant(), time.Since(bm25Started).Milliseconds())
		}
	} else {
		sources = vectorSources
	}
	sources = s.filterAvailableSources(ctx, sources)
	sources = s.resolveParentTexts(ctx, sources)
	if len(sources) > topK {
		sources = sources[:topK]
	}
	// Only normalize when RRF fusion was used (produces small rank-based scores).
	// Raw cosine similarity scores from Chroma are already in [0,1] with intuitive meaning.
	if s.hybridSearch() {
		sources = normalizeScores(sources)
	}
	log.Printf("level=info component=search event=query_complete candidates=%d results=%d candidate_top_k=%d query_duration_ms=%d duration_ms=%d",
		queryResultLen(result), len(sources), candidateTopK, time.Since(queryStarted).Milliseconds(), time.Since(started).Milliseconds())
	return sources, nil
}

func (s *SearchService) searchMulti(ctx context.Context, queries []string, fileIDs []string, topK int) ([]SourceChunk, error) {
	if len(queries) == 0 {
		return nil, nil
	}
	if len(queries) == 1 {
		return s.searchSingle(ctx, queries[0], fileIDs, topK)
	}

	type result struct {
		sources []SourceChunk
		err     error
	}
	results := make([]result, len(queries))
	var wg sync.WaitGroup
	for i, query := range queries {
		wg.Add(1)
		go func(index int, q string) {
			defer wg.Done()
			sources, err := s.searchSingle(ctx, q, fileIDs, topK)
			results[index] = result{sources: sources, err: err}
		}(i, query)
	}
	wg.Wait()

	var firstErr error
	errorCount := 0
	byID := map[string]SourceChunk{}
	for _, item := range results {
		if item.err != nil {
			errorCount++
			if firstErr == nil {
				firstErr = item.err
			}
			continue
		}
		for _, source := range item.sources {
			if existing, ok := byID[source.ID]; ok && existing.Score >= source.Score {
				continue
			}
			byID[source.ID] = source
		}
	}
	if len(byID) == 0 && firstErr != nil {
		return nil, firstErr
	}
	merged := make([]SourceChunk, 0, len(byID))
	for _, source := range byID {
		merged = append(merged, source)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})
	if len(merged) > topK {
		merged = merged[:topK]
	}
	log.Printf("level=info component=search event=multi_query_merge queries=%d results=%d errors=%d",
		len(queries), len(merged), errorCount)
	return merged, nil
}

func (s *SearchService) mapResults(result *vectordb.QueryResult, fileIDs []string, topK int) []SourceChunk {
	if result == nil || topK <= 0 {
		return nil
	}
	fileFilter := fileIDSet(fileIDs)
	minScore := s.minScore()
	limit := queryResultLen(result)
	sources := make([]SourceChunk, 0, minInt(limit, topK))
	for i := 0; i < limit; i++ {
		source := sourceFromQueryResult(result, i)
		if len(fileFilter) > 0 {
			if _, ok := fileFilter[source.FileID]; !ok {
				continue
			}
		}
		if minScore > 0 && source.Score < minScore {
			continue
		}
		sources = append(sources, source)
	}
	beforePercentile := len(sources)
	sources = s.applyScorePercentile(sources)
	if beforePercentile != len(sources) {
		log.Printf("level=info component=search event=percentile_filter percentile=%.2f before=%d after=%d",
			s.scorePercentile(), beforePercentile, len(sources))
	}
	logScoreDistribution("vector", sources)
	if len(sources) > topK {
		sources = sources[:topK]
	}
	return sources
}

func sourceFromQueryResult(result *vectordb.QueryResult, index int) SourceChunk {
	metadata := metadataAt(result.Metadatas, index)
	source := SourceChunk{
		ID:         stringAt(result.IDs, index),
		FileID:     metadataString(metadata, "file_id"),
		FileName:   metadataString(metadata, "file_name"),
		Heading:    metadataString(metadata, "heading"),
		ChunkIndex: metadataInt(metadata, "chunk_index", -1),
		ParentID:   metadataString(metadata, vectordb.MetadataParentChunkID),
		Text:       stringAt(result.Documents, index),
		Distance:   float32At(result.Distances, index),
	}
	source.Score = 1 - source.Distance
	if source.Score < 0 {
		source.Score = 0
	}
	if source.Score > 1 {
		source.Score = 1
	}
	source.Snippet = makeSnippet(source.Text, defaultSnippetLength)
	if source.FileID == "" || source.FileName == "" || source.ChunkIndex < 0 {
		log.Printf("level=warn component=search event=metadata_incomplete chunk_id=%q file_id=%q file_name=%q chunk_index=%d", source.ID, source.FileID, source.FileName, source.ChunkIndex)
	}
	return source
}

func metadataAt(items []map[string]any, index int) map[string]any {
	if index < 0 || index >= len(items) || items[index] == nil {
		return map[string]any{}
	}
	return items[index]
}

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func metadataInt(metadata map[string]any, key string, fallback int) int {
	value, ok := metadata[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		if uint64(typed) > uint64(maxIntValue()) {
			return fallback
		}
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		if typed > uint64(maxIntValue()) {
			return fallback
		}
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
		if parsed, err := typed.Float64(); err == nil {
			return int(parsed)
		}
	case string:
		var parsed json.Number = json.Number(strings.TrimSpace(typed))
		if value, err := parsed.Int64(); err == nil {
			return int(value)
		}
	}
	return fallback
}

func makeSnippet(text string, limit int) string {
	text = strings.TrimSpace(text)
	if limit <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return strings.TrimSpace(string(runes[:limit])) + "..."
}

func (s *SearchService) resolveParentTexts(ctx context.Context, sources []SourceChunk) []SourceChunk {
	if s == nil || s.store == nil || len(sources) == 0 {
		return sources
	}
	for i, source := range sources {
		parentID := strings.TrimSpace(source.ParentID)
		if parentID == "" {
			continue
		}
		parentText, err := s.store.GetChunkText(ctx, parentID)
		if err != nil || strings.TrimSpace(parentText) == "" {
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				log.Printf("level=warn component=search event=parent_lookup_failed chunk_id=%q parent_chunk_id=%q err=%q", source.ID, parentID, err)
			}
			continue
		}
		sources[i].Text = parentText
		sources[i].Snippet = makeSnippet(parentText, defaultSnippetLength)
	}
	return sources
}

func (s *SearchService) filterAvailableSources(ctx context.Context, sources []SourceChunk) []SourceChunk {
	if s == nil || s.store == nil || len(sources) == 0 {
		return sources
	}
	available := map[string]bool{}
	filtered := sources[:0]
	for _, source := range sources {
		fileID := strings.TrimSpace(source.FileID)
		if fileID == "" {
			filtered = append(filtered, source)
			continue
		}
		ok, seen := available[fileID]
		if !seen {
			_, err := s.store.GetFile(ctx, fileID)
			ok = err == nil
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				log.Printf("level=warn component=search event=source_file_lookup_failed file_id=%s chunk_id=%q err=%q", fileID, source.ID, err)
			}
			available[fileID] = ok
		}
		if ok {
			filtered = append(filtered, source)
			continue
		}
		log.Printf("level=debug component=search event=source_file_skipped file_id=%s chunk_id=%q reason=missing_or_deleted", fileID, source.ID)
	}
	return filtered
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

func percentileScore(sources []SourceChunk, percentile float32) float32 {
	if len(sources) == 0 {
		return 0
	}
	scores := make([]float32, len(sources))
	for i, source := range sources {
		scores[i] = source.Score
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i] < scores[j]
	})
	index := int(float32(len(scores)) * percentile)
	if index < 0 {
		index = 0
	}
	if index >= len(scores) {
		index = len(scores) - 1
	}
	return scores[index]
}

func rrfFusion(vectorResults, bm25Results []SourceChunk, k int) []SourceChunk {
	if k <= 0 {
		k = defaultRRFK
	}
	scores := map[string]float64{}
	chunks := map[string]SourceChunk{}
	for rank, source := range vectorResults {
		if source.ID == "" {
			continue
		}
		scores[source.ID] += 1.0 / float64(k+rank+1)
		chunks[source.ID] = source
	}
	for rank, source := range bm25Results {
		if source.ID == "" {
			continue
		}
		scores[source.ID] += 1.0 / float64(k+rank+1)
		if _, ok := chunks[source.ID]; !ok {
			chunks[source.ID] = source
		}
	}
	merged := make([]SourceChunk, 0, len(chunks))
	for id, source := range chunks {
		source.Score = float32(scores[id])
		merged = append(merged, source)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i].Score > merged[j].Score
	})
	return merged
}

func logScoreDistribution(scope string, sources []SourceChunk) {
	if len(sources) == 0 {
		return
	}
	scores := make([]float32, len(sources))
	for i, source := range sources {
		scores[i] = source.Score
	}
	sort.Slice(scores, func(i, j int) bool {
		return scores[i] < scores[j]
	})
	log.Printf("level=info component=search event=score_distribution scope=%s min=%.4f max=%.4f median=%.4f count=%d",
		scope, scores[0], scores[len(scores)-1], scores[len(scores)/2], len(scores))
}
