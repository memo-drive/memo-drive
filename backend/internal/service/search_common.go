package service

import (
	"strings"

	"github.com/memodrive/backend/internal/vectordb"
)

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
