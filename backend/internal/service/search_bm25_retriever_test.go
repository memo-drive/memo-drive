package service

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestBM25ChunkRetrieverSearchesChunkStore(t *testing.T) {
	store := &recordingBM25ChunkStore{
		sources: []SourceChunk{{ID: "file-1#0", FileID: "file-1", Score: 1}},
	}
	retriever := bm25ChunkRetriever{Store: store}

	result, err := retriever.Retrieve(context.Background(), bm25ChunkRetrievalOptions{
		Query:   "localOnly",
		FileIDs: []string{"file-1"},
		Limit:   12,
	})
	if err != nil {
		t.Fatalf("Retrieve returned error: %v", err)
	}

	if store.query != "localOnly" || !reflect.DeepEqual(store.fileIDs, []string{"file-1"}) || store.limit != 12 {
		t.Fatalf("unexpected store call: %#v", store)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != "file-1#0" {
		t.Fatalf("unexpected sources: %#v", result.Sources)
	}
}

type recordingBM25ChunkStore struct {
	query   string
	fileIDs []string
	limit   int
	sources []SourceChunk
	err     error
}

func (s *recordingBM25ChunkStore) SearchChunksBM25(_ context.Context, query string, fileIDs []string, limit int) ([]SourceChunk, error) {
	s.query = query
	s.fileIDs = append([]string(nil), fileIDs...)
	s.limit = limit
	if s.err != nil {
		return nil, s.err
	}
	return append([]SourceChunk(nil), s.sources...), nil
}

func TestBM25ChunkRetrieverReturnsStoreError(t *testing.T) {
	wantErr := errors.New("bm25 failed")
	retriever := bm25ChunkRetriever{Store: &recordingBM25ChunkStore{err: wantErr}}

	_, err := retriever.Retrieve(context.Background(), bm25ChunkRetrievalOptions{Query: "localOnly", Limit: 10})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected store error, got %v", err)
	}
}
