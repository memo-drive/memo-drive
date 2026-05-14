package service

import (
	"context"
	"testing"

	"github.com/memodrive/backend/internal/vectordb"
)

func TestVectorChunkRetrieverEmbedsQueriesVectorIndexAndMapsSources(t *testing.T) {
	provider := &mockSearchProvider{}
	vector := &mockVectorStore{queryResult: sampleQueryResult()}
	retriever := vectorChunkRetriever{
		Embedder:   provider,
		Store:      vector,
		Collection: vectordb.DefaultCollection,
		Mapper:     vectorChunkMapper{},
	}

	result, err := retriever.Retrieve(context.Background(), vectorChunkRetrievalOptions{
		Query:         "login issue",
		FileIDs:       []string{"file-b"},
		CandidateTopK: minCandidateTopK,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if provider.embedCalls != 1 || len(provider.embedTexts) != 1 || provider.embedTexts[0] != "login issue" {
		t.Fatalf("expected query to be embedded once, got calls=%d texts=%#v", provider.embedCalls, provider.embedTexts)
	}
	if !vector.queryCalled || vector.queryNResults != minCandidateTopK {
		t.Fatalf("expected vector query candidate limit %d, got called=%t n=%d", minCandidateTopK, vector.queryCalled, vector.queryNResults)
	}
	if result.Candidates != 2 || result.Dimensions != 3 {
		t.Fatalf("unexpected retrieval metrics: %#v", result)
	}
	if len(result.Sources) != 1 || result.Sources[0].FileID != "file-b" {
		t.Fatalf("expected mapped and filtered file-b source, got %#v", result.Sources)
	}
}
