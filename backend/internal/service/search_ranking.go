package service

import (
	"context"
	"errors"
	"log"
	"sort"
	"strings"

	"github.com/memodrive/backend/internal/store"
)

type chunkRankingOptions struct {
	TopK            int
	NormalizeScores bool
}

// rankChunkEvidence post-processes search results in three passes:
//  1. filterAvailableSources — removes chunks whose parent file no longer exists in the DB
//  2. resolveParentTexts — replaces child chunk text with the richer parent chunk text
//     (child chunks are small and optimized for vector matching; parent chunks provide context)
//  3. truncate to TopK, optionally normalize scores to [0, 1]
func (s *SearchService) rankChunkEvidence(ctx context.Context, sources []SourceChunk, opts chunkRankingOptions) []SourceChunk {
	sources = s.filterAvailableSources(ctx, sources)
	sources = s.resolveParentTexts(ctx, sources)
	if len(sources) > opts.TopK {
		sources = sources[:opts.TopK]
	}
	if opts.NormalizeScores {
		sources = normalizeScores(sources)
	}
	return sources
}

// resolveParentTexts implements the "parent document retrieval" pattern.
// Each child chunk has a ParentChunkID pointing to a larger parent chunk.
// After vector search finds relevant child chunks (small, precise), we swap
// their text with the parent chunk text (large, context-rich) for better RAG quality.
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
