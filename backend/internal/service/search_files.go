package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

// SearchFiles performs file-level search across file names, metadata, and optionally
// semantic vector search. Results are merged by file ID with multi-match scoring.
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
