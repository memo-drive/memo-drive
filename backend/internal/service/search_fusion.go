package service

import "sort"

type chunkEvidenceFusion struct {
	RRFConstant int
}

type chunkQueryResult struct {
	Sources []SourceChunk
	Err     error
}

type chunkQueryMergeResult struct {
	Sources    []SourceChunk
	ErrorCount int
}

func (f chunkEvidenceFusion) FuseHybrid(vectorResults, bm25Results []SourceChunk) []SourceChunk {
	k := f.RRFConstant
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

func (f chunkEvidenceFusion) MergeMultiQuery(results []chunkQueryResult, topK int) (chunkQueryMergeResult, error) {
	var firstErr error
	errorCount := 0
	byID := map[string]SourceChunk{}
	for _, item := range results {
		if item.Err != nil {
			errorCount++
			if firstErr == nil {
				firstErr = item.Err
			}
			continue
		}
		for _, source := range item.Sources {
			if existing, ok := byID[source.ID]; ok && existing.Score >= source.Score {
				continue
			}
			byID[source.ID] = source
		}
	}
	if len(byID) == 0 && firstErr != nil {
		return chunkQueryMergeResult{ErrorCount: errorCount}, firstErr
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
	return chunkQueryMergeResult{
		Sources:    merged,
		ErrorCount: errorCount,
	}, nil
}
