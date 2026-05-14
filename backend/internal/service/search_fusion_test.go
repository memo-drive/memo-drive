package service

import (
	"errors"
	"testing"
)

func TestChunkEvidenceFusionFusesHybridResultsWithRRF(t *testing.T) {
	fusion := chunkEvidenceFusion{RRFConstant: 60}
	vector := []SourceChunk{
		{ID: "shared", Text: "vector shared", Score: 0.8},
		{ID: "vector-only", Text: "vector only", Score: 0.7},
	}
	bm25 := []SourceChunk{
		{ID: "bm25-only", Text: "bm25 only", Score: 1},
		{ID: "shared", Text: "bm25 shared", Score: 1},
	}

	results := fusion.FuseHybrid(vector, bm25)

	if len(results) != 3 {
		t.Fatalf("expected three fused results, got %#v", results)
	}
	if results[0].ID != "shared" {
		t.Fatalf("expected shared chunk to rank first, got %#v", results)
	}
	if results[0].Text != "vector shared" {
		t.Fatalf("expected vector source to be preserved for shared chunk, got %#v", results[0])
	}
	if results[0].Score <= results[1].Score {
		t.Fatalf("expected shared score to beat single-source scores, got %#v", results)
	}
}

func TestChunkEvidenceFusionMergesMultiQueryResults(t *testing.T) {
	queryErr := errors.New("query failed")
	result, err := (chunkEvidenceFusion{}).MergeMultiQuery([]chunkQueryResult{
		{Sources: []SourceChunk{{ID: "shared", Score: 0.4}, {ID: "low", Score: 0.2}}},
		{Sources: []SourceChunk{{ID: "shared", Score: 0.9}, {ID: "high", Score: 0.8}}},
		{Err: queryErr},
	}, 2)

	if err != nil {
		t.Fatalf("expected partial query errors to be tolerated, got %v", err)
	}
	if result.ErrorCount != 1 {
		t.Fatalf("expected one partial error, got %#v", result)
	}
	if len(result.Sources) != 2 {
		t.Fatalf("expected top two merged results, got %#v", result.Sources)
	}
	if result.Sources[0].ID != "shared" || result.Sources[0].Score != 0.9 {
		t.Fatalf("expected duplicate to keep highest score first, got %#v", result.Sources)
	}
	if result.Sources[1].ID != "high" {
		t.Fatalf("expected high-scoring unique result second, got %#v", result.Sources)
	}
}

func TestChunkEvidenceFusionReturnsFirstErrorWhenAllMultiQueryResultsFail(t *testing.T) {
	firstErr := errors.New("first")
	_, err := (chunkEvidenceFusion{}).MergeMultiQuery([]chunkQueryResult{
		{Err: firstErr},
		{Err: errors.New("second")},
	}, 5)

	if !errors.Is(err, firstErr) {
		t.Fatalf("expected first error, got %v", err)
	}
}
