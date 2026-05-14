package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/store"
)

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

func (s *SearchService) vectorChunkMapper() vectorChunkMapper {
	return vectorChunkMapper{
		MinScore:        s.minScore(),
		ScorePercentile: s.scorePercentile(),
	}
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

func (s *SearchService) chunkEvidenceFusion() chunkEvidenceFusion {
	return chunkEvidenceFusion{RRFConstant: s.rrfConstant()}
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
