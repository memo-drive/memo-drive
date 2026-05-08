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

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
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
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, ErrEmptyQuery
	}
	if s == nil || s.llm == nil {
		return nil, fmt.Errorf("%w: llm provider is not configured", ErrServiceUnavailable)
	}
	if s.vectorDB == nil {
		return nil, fmt.Errorf("%w: vector store is not configured", ErrServiceUnavailable)
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
				return &SearchResponse{Query: effectiveIntentQuery(req.Query, parsed), Intent: intent}, nil
			} else {
				req.FileIDs = constrainFileIDs(req.FileIDs, ids)
				if len(req.FileIDs) == 0 {
					return &SearchResponse{Query: effectiveIntentQuery(req.Query, parsed), Intent: intent}, nil
				}
			}
		}
		query = effectiveIntentQuery(req.Query, parsed)
	}

	topK := s.searchTopK(req.TopK)
	queries := s.expandQueries(ctx, query, s.multiQueryCount())
	log.Printf("level=info component=search event=search_begin query_chars=%d top_k=%d file_filter=%d provider=%s queries=%d hybrid=%t",
		len([]rune(query)), topK, len(req.FileIDs), s.llm.Name(), len(queries), s.hybridSearch())

	sources, err := s.searchMulti(ctx, queries, req.FileIDs, topK)
	if err != nil {
		log.Printf("level=error component=search event=search_failed query_chars=%d duration_ms=%d err=%q", len([]rune(query)), time.Since(started).Milliseconds(), err)
		return nil, err
	}
	logScoreDistribution("search", sources)
	log.Printf("level=info component=search event=search_complete results=%d queries=%d duration_ms=%d",
		len(sources), len(queries), time.Since(started).Milliseconds())

	return &SearchResponse{
		Query:   query,
		Results: sources,
		Intent:  intent,
	}, nil
}

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

func (s *SearchService) expandQueries(ctx context.Context, query string, count int) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	if s == nil || s.llm == nil || !s.multiQueryEnabled() {
		return []string{query}
	}
	if count <= 0 {
		count = defaultMultiQueryCount
	}
	prompt := fmt.Sprintf(multiQueryPrompt, count, query)
	result, err := s.llm.Complete(ctx, []llm.Message{{Role: "user", Content: prompt}})
	if err != nil {
		log.Printf("level=warn component=search event=multi_query_failed query_chars=%d err=%q", len([]rune(query)), err)
		return []string{query}
	}
	queries := []string{query}
	seen := map[string]struct{}{strings.ToLower(query): {}}
	for _, line := range strings.Split(result, "\n") {
		line = cleanGeneratedQuery(line)
		if line == "" {
			continue
		}
		key := strings.ToLower(line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		queries = append(queries, line)
		if len(queries) >= count+1 {
			break
		}
	}
	log.Printf("level=info component=search event=multi_query_expand original_chars=%d variants=%d", len([]rune(query)), len(queries)-1)
	return queries
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

func (s *SearchService) SearchFiles(ctx context.Context, req FileSearchRequest) (*FileSearchResponse, error) {
	started := time.Now()
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, ErrEmptyQuery
	}
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("%w: file store is not configured", ErrServiceUnavailable)
	}

	limit := fileSearchLimit(req.Limit)
	var intent *SearchIntent
	if s.intentParseEnabled() {
		parsed := s.parseIntent(ctx, query)
		intent = &parsed
		if parsed.TextQuery != "" || parsed.HasFilters() {
			query = parsed.TextQuery
		}
	}
	filter := store.FileSearchFilter{
		Keyword:    query,
		PathPrefix: cleanSearchPath(req.Path),
		MimePrefix: strings.TrimSpace(req.MimePrefix),
		Limit:      limit,
	}
	if intent != nil && intent.HasFilters() {
		if filter.MimePrefix == "" {
			filter.MimePrefix = intent.PrimaryMime()
		}
		filter.Extensions = intent.Extensions
		filter.DateFrom = intent.DateFrom
		filter.DateTo = intent.DateTo
	}
	semanticRequested := req.Semantic
	semanticEnabled := semanticRequested && s.llm != nil && s.vectorDB != nil
	log.Printf("level=info component=search event=file_search_begin query_chars=%d path=%q mime=%q semantic=%t",
		len([]rune(query)), filter.PathPrefix, filter.MimePrefix, semanticEnabled)
	if semanticRequested && !semanticEnabled {
		log.Printf("level=warn component=search event=file_search_semantic_skipped reason=dependency_unavailable llm=%t vector_db=%t", s.llm != nil, s.vectorDB != nil)
	}

	var (
		nameHits     []model.File
		metaHits     []store.MetadataHit
		semanticHits []SourceChunk
		nameErr      error
		metaErr      error
		semanticErr  error
		wg           sync.WaitGroup
	)
	if filter.Keyword != "" {
		wg.Add(2)
		go func() {
			defer wg.Done()
			nameHits, nameErr = s.store.SearchFilesByName(ctx, filter)
		}()
		go func() {
			defer wg.Done()
			metaHits, metaErr = s.store.SearchFilesByMetadata(ctx, filter)
		}()
	}
	if semanticEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			topK := minInt(maxSearchTopK, maxInt(limit, minCandidateTopK))
			semanticQuery := query
			if semanticQuery == "" {
				semanticQuery = strings.TrimSpace(req.Query)
			}
			var fileIDs []string
			if filter.HasStructuredFilters() || filter.PathPrefix != "" {
				ids, err := s.store.ListFileIDsByFilter(ctx, store.FileSearchFilter{
					PathPrefix: filter.PathPrefix,
					MimePrefix: filter.MimePrefix,
					Extensions: filter.Extensions,
					DateFrom:   filter.DateFrom,
					DateTo:     filter.DateTo,
					Limit:      s.intentFileLimit(),
				})
				if err != nil {
					semanticErr = err
					return
				}
				if len(ids) == 0 {
					semanticHits = nil
					return
				}
				fileIDs = ids
			}
			response, err := s.Search(ctx, SearchRequest{Query: semanticQuery, FileIDs: fileIDs, TopK: topK})
			if err != nil {
				semanticErr = err
				return
			}
			semanticHits = response.Results
		}()
	}
	wg.Wait()

	if nameErr != nil {
		return nil, fmt.Errorf("search files by name: %w", nameErr)
	}
	if metaErr != nil {
		return nil, fmt.Errorf("search files by metadata: %w", metaErr)
	}
	if semanticErr != nil {
		log.Printf("level=warn component=search event=file_search_semantic_failed query_chars=%d err=%q", len([]rune(query)), semanticErr)
		semanticEnabled = false
		semanticHits = nil
	}

	hitsByID := map[string]*FileSearchHit{}
	baseScores := map[string]float32{}
	for i := range nameHits {
		upsertFileSearchHit(hitsByID, baseScores, &nameHits[i], "name", 1, "")
	}
	for i := range metaHits {
		upsertFileSearchHit(hitsByID, baseScores, &metaHits[i].File, "meta", 0.7, metaHits[i].Snippet)
	}
	if filter.Keyword == "" && filter.HasStructuredFilters() {
		filterIDs, err := s.store.ListFileIDsByFilter(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("list files by intent filter: %w", err)
		}
		for _, id := range filterIDs {
			file, err := s.store.GetFile(ctx, id)
			if err != nil {
				if errors.Is(err, store.ErrNotFound) {
					continue
				}
				return nil, fmt.Errorf("load filter hit file %s: %w", id, err)
			}
			upsertFileSearchHit(hitsByID, baseScores, file, "filter", 1, "")
		}
	}
	for _, source := range semanticHits {
		if source.FileID == "" {
			continue
		}
		file, err := s.store.GetFile(ctx, source.FileID)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				log.Printf("level=warn component=search event=file_search_semantic_file_missing file_id=%s chunk_id=%q", source.FileID, source.ID)
				continue
			}
			return nil, fmt.Errorf("load semantic hit file %s: %w", source.FileID, err)
		}
		if !fileMatchesSearchFilter(file, filter) {
			continue
		}
		upsertFileSearchHit(hitsByID, baseScores, file, "semantic", source.Score, source.Snippet)
	}

	hits := make([]FileSearchHit, 0, len(hitsByID))
	for id, hit := range hitsByID {
		sortMatchTypes(hit.MatchTypes)
		hit.Score = clampScore(baseScores[id] + float32(len(hit.MatchTypes)-1)*0.05)
		hits = append(hits, *hit)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		if hits[i].File == nil || hits[j].File == nil {
			return hits[i].File != nil
		}
		return hits[i].File.UpdatedAt.After(hits[j].File.UpdatedAt)
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}

	responseQuery := query
	if intent != nil {
		responseQuery = effectiveIntentQuery(req.Query, *intent)
	}
	log.Printf("level=info component=search event=file_search_complete hits_total=%d name=%d meta=%d semantic=%d duration_ms=%d",
		len(hits), len(nameHits), len(metaHits), len(semanticHits), time.Since(started).Milliseconds())
	return &FileSearchResponse{
		Query:    responseQuery,
		Total:    len(hits),
		Hits:     hits,
		Semantic: semanticEnabled,
		Intent:   intent,
	}, nil
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

func (s *SearchService) searchTopK(requested int) int {
	if requested <= 0 {
		if s != nil && s.cfg != nil && s.cfg.RAG.SearchTopK > 0 {
			requested = s.cfg.RAG.SearchTopK
		} else {
			requested = defaultSearchTopK
		}
	}
	if requested > maxSearchTopK {
		return maxSearchTopK
	}
	return requested
}

func (s *SearchService) minScore() float32 {
	if s == nil || s.cfg == nil {
		return 0
	}
	return s.cfg.RAG.MinScore
}

func (s *SearchService) scorePercentile() float32 {
	if s == nil || s.cfg == nil {
		return 0
	}
	return s.cfg.RAG.ScorePercentile
}

func (s *SearchService) multiQueryEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.RAG.MultiQuery
}

func (s *SearchService) multiQueryCount() int {
	if s == nil || s.cfg == nil || s.cfg.RAG.MultiQueryCount <= 0 {
		return defaultMultiQueryCount
	}
	return s.cfg.RAG.MultiQueryCount
}

func (s *SearchService) hybridSearch() bool {
	return s != nil && s.cfg != nil && s.cfg.RAG.HybridSearch
}

func (s *SearchService) rrfConstant() int {
	if s == nil || s.cfg == nil || s.cfg.RAG.RRFConstant <= 0 {
		return defaultRRFK
	}
	return s.cfg.RAG.RRFConstant
}

func (s *SearchService) intentParseEnabled() bool {
	return s != nil && s.cfg != nil && s.cfg.RAG.IntentParse
}

func (s *SearchService) intentFileLimit() int {
	if s == nil || s.cfg == nil || s.cfg.RAG.IntentFileLimit <= 0 {
		return 500
	}
	return s.cfg.RAG.IntentFileLimit
}

func (s *SearchService) parseIntent(ctx context.Context, query string) SearchIntent {
	opts := SearchIntentOptions{
		Now:         time.Now(),
		Timezone:    "Asia/Shanghai",
		LLMFallback: true,
	}
	if s != nil && s.cfg != nil {
		opts.Timezone = s.cfg.RAG.IntentTimezone
		opts.LLMFallback = s.cfg.RAG.IntentLLMFallback
	}
	return ParseSearchIntentWithOptions(ctx, query, s.llm, opts)
}

func (s *SearchService) fileIDsForIntent(ctx context.Context, intent SearchIntent, base store.FileSearchFilter) ([]string, error) {
	if s == nil || s.store == nil || !intent.HasFilters() {
		return nil, nil
	}
	filter := store.FileSearchFilter{
		PathPrefix: base.PathPrefix,
		MimePrefix: base.MimePrefix,
		Extensions: intent.Extensions,
		DateFrom:   intent.DateFrom,
		DateTo:     intent.DateTo,
		Limit:      s.intentFileLimit(),
	}
	if filter.MimePrefix == "" {
		filter.MimePrefix = intent.PrimaryMime()
	}
	return s.store.ListFileIDsByFilter(ctx, filter)
}

func (s *SearchService) applyScorePercentile(sources []SourceChunk) []SourceChunk {
	percentile := s.scorePercentile()
	if percentile <= 0 || len(sources) <= 2 {
		return sources
	}
	threshold := percentileScore(sources, percentile)
	filtered := sources[:0]
	for _, source := range sources {
		if source.Score >= threshold {
			filtered = append(filtered, source)
		}
	}
	return filtered
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

func cleanGeneratedQuery(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimLeft(query, "-*• \t")
	query = strings.TrimSpace(query)
	for i, r := range query {
		if (r == '.' || r == '、' || r == ')' || r == '）') && i > 0 && i <= 4 {
			prefix := strings.TrimSpace(query[:i])
			var number int
			if _, err := fmt.Sscanf(prefix, "%d", &number); err == nil {
				return strings.TrimSpace(query[i+len(string(r)):])
			}
		}
	}
	return query
}

func fileIDSet(fileIDs []string) map[string]struct{} {
	set := make(map[string]struct{}, len(fileIDs))
	for _, id := range fileIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			set[id] = struct{}{}
		}
	}
	return set
}

func constrainFileIDs(existing, candidates []string) []string {
	candidates = uniqueStrings(candidates)
	if len(existing) == 0 {
		return candidates
	}
	candidateSet := fileIDSet(candidates)
	constrained := make([]string, 0, minInt(len(existing), len(candidates)))
	for _, id := range existing {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := candidateSet[id]; ok {
			constrained = append(constrained, id)
		}
	}
	return uniqueStrings(constrained)
}

func effectiveIntentQuery(original string, intent SearchIntent) string {
	textQuery := strings.TrimSpace(intent.TextQuery)
	if textQuery != "" {
		return textQuery
	}
	return strings.TrimSpace(original)
}

func queryResultLen(result *vectordb.QueryResult) int {
	if result == nil {
		return 0
	}
	limit := len(result.IDs)
	limit = minInt(limit, len(result.Documents))
	limit = minInt(limit, len(result.Distances))
	return limit
}

func stringAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func float32At(values []float32, index int) float32 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func maxIntValue() int {
	return int(^uint(0) >> 1)
}

func fileSearchLimit(requested int) int {
	if requested <= 0 {
		return defaultFileLimit
	}
	if requested > maxFileLimit {
		return maxFileLimit
	}
	return requested
}

func cleanSearchPath(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return strings.TrimRight(value, "/")
}

func fileMatchesSearchFilter(file *model.File, filter store.FileSearchFilter) bool {
	if file == nil || file.IsDir {
		return false
	}
	if filter.PathPrefix != "" && file.Path != filter.PathPrefix && !strings.HasPrefix(file.Path, filter.PathPrefix+"/") {
		return false
	}
	if filter.MimePrefix != "" && !strings.HasPrefix(file.MimeType, filter.MimePrefix) {
		return false
	}
	if len(filter.Extensions) > 0 {
		name := strings.ToLower(file.Name)
		matched := false
		for _, ext := range filter.Extensions {
			ext = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ext)), ".")
			if ext != "" && strings.HasSuffix(name, "."+ext) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if filter.DateFrom != nil && file.CreatedAt.Before(filter.DateFrom.UTC()) {
		return false
	}
	if filter.DateTo != nil && file.CreatedAt.After(filter.DateTo.UTC()) {
		return false
	}
	return true
}

func upsertFileSearchHit(hits map[string]*FileSearchHit, baseScores map[string]float32, file *model.File, matchType string, score float32, snippet string) {
	if file == nil || file.ID == "" {
		return
	}
	hit, ok := hits[file.ID]
	if !ok {
		fileCopy := *file
		hit = &FileSearchHit{File: &fileCopy}
		hits[file.ID] = hit
	}
	if !containsString(hit.MatchTypes, matchType) {
		hit.MatchTypes = append(hit.MatchTypes, matchType)
	}
	if score > baseScores[file.ID] {
		baseScores[file.ID] = score
	}
	if len([]rune(snippet)) > len([]rune(hit.Snippet)) {
		hit.Snippet = snippet
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sortMatchTypes(values []string) {
	order := map[string]int{"name": 0, "meta": 1, "semantic": 2, "filter": 3}
	sort.SliceStable(values, func(i, j int) bool {
		return order[values[i]] < order[values[j]]
	})
}

func clampScore(score float32) float32 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func normalizeScores(sources []SourceChunk) []SourceChunk {
	if len(sources) == 0 {
		return sources
	}
	var maxScore float32
	for _, s := range sources {
		if s.Score > maxScore {
			maxScore = s.Score
		}
	}
	if maxScore <= 0 {
		return sources
	}
	for i := range sources {
		sources[i].Score = clampScore(sources[i].Score / maxScore)
	}
	return sources
}
