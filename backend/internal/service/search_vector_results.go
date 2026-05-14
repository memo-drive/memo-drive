package service

import (
	"log"

	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/vectordb"
)

type vectorChunkMapper struct {
	MinScore        float32
	ScorePercentile float32
}

type vectorChunkMappingOptions struct {
	FileIDs []string
	TopK    int
}

func (m vectorChunkMapper) Map(result *vectordb.QueryResult, opts vectorChunkMappingOptions) []SourceChunk {
	if result == nil || opts.TopK <= 0 {
		return nil
	}
	fileFilter := fileIDSet(opts.FileIDs)
	limit := queryResultLen(result)
	sources := make([]SourceChunk, 0, minInt(limit, opts.TopK))
	for i := 0; i < limit; i++ {
		source := sourceFromQueryResult(result, i)
		if len(fileFilter) > 0 {
			if _, ok := fileFilter[source.FileID]; !ok {
				continue
			}
		}
		if m.MinScore > 0 && source.Score < m.MinScore {
			continue
		}
		sources = append(sources, source)
	}
	beforePercentile := len(sources)
	sources = applyVectorScorePercentile(sources, m.ScorePercentile)
	if beforePercentile != len(sources) {
		log.Printf("level=info component=search event=percentile_filter percentile=%.2f before=%d after=%d",
			m.ScorePercentile, beforePercentile, len(sources))
	}
	logScoreDistribution("vector", sources)
	if len(sources) > opts.TopK {
		sources = sources[:opts.TopK]
	}
	return sources
}

func sourceFromQueryResult(result *vectordb.QueryResult, index int) SourceChunk {
	metadata := indexing.ChunkMetadataFromMap(metadataAt(result.Metadatas, index))
	source := SourceChunk{
		ID:         stringAt(result.IDs, index),
		FileID:     metadata.FileID,
		FileName:   metadata.FileName,
		Heading:    metadata.Heading,
		ChunkIndex: metadata.ChunkIndex,
		ParentID:   metadata.ParentChunkID,
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

func applyVectorScorePercentile(sources []SourceChunk, percentile float32) []SourceChunk {
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
