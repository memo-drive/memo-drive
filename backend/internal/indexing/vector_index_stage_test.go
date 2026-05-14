package indexing

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestVectorIndexStageEmbedsPlanAndUpsertsVectors(t *testing.T) {
	plan := DocumentIndexPlan{
		VectorIDs:   []string{ChunkID("file-1", 0), ChunkID("file-1", 1)},
		VectorTexts: []string{"first chunk", "second chunk"},
		VectorMetadatas: []map[string]any{
			(ChunkMetadata{FileID: "file-1", FileName: "Guide.md", ChunkIndex: 0}).Map(),
			(ChunkMetadata{FileID: "file-1", FileName: "Guide.md", ChunkIndex: 1}).Map(),
		},
	}
	embeddings := [][]float32{{1, 0}, {0, 1}}
	embedder := &recordingVectorStageEmbedder{embeddings: embeddings}
	store := &recordingVectorStageStore{}

	result, err := (VectorIndexStage{
		Embedder:   embedder,
		Store:      store,
		Collection: "memodrive",
		BatchSize:  10,
	}).Run(context.Background(), plan)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	if !reflect.DeepEqual(embedder.calls, [][]string{plan.VectorTexts}) {
		t.Fatalf("expected plan texts to be embedded, got %#v", embedder.calls)
	}
	if store.collection != "memodrive" ||
		!reflect.DeepEqual(store.ids, plan.VectorIDs) ||
		!reflect.DeepEqual(store.embeddings, embeddings) ||
		!reflect.DeepEqual(store.documents, plan.VectorTexts) ||
		!reflect.DeepEqual(store.metadatas, plan.VectorMetadatas) {
		t.Fatalf("unexpected vector upsert: %#v", store)
	}
	if result.Count != 2 || result.Dimensions != 2 {
		t.Fatalf("unexpected vector index result: %#v", result)
	}
}

func TestVectorIndexStageBatchesEmbeddings(t *testing.T) {
	embedder := &recordingVectorStageEmbedder{}

	embeddings, err := (VectorIndexStage{
		Embedder:  embedder,
		BatchSize: 2,
	}).EmbedTexts(context.Background(), []string{"a", "b", "c", "d", "e"})
	if err != nil {
		t.Fatalf("EmbedTexts returned error: %v", err)
	}

	if len(embeddings) != 5 {
		t.Fatalf("expected 5 embeddings, got %d", len(embeddings))
	}
	if len(embedder.calls) != 3 {
		t.Fatalf("expected 3 batches, got %#v", embedder.calls)
	}
	if len(embedder.calls[0]) != 2 || len(embedder.calls[2]) != 1 {
		t.Fatalf("unexpected batch sizes: %#v", embedder.calls)
	}
}

func TestVectorIndexStageRetriesEmbeddingBatch(t *testing.T) {
	embedder := &recordingVectorStageEmbedder{failures: 1}

	embeddings, err := (VectorIndexStage{
		Embedder:      embedder,
		BatchSize:     2,
		EmbedAttempts: 2,
	}).EmbedTexts(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("EmbedTexts returned error after retry: %v", err)
	}

	if len(embeddings) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(embeddings))
	}
	if len(embedder.calls) != 2 {
		t.Fatalf("expected failed call plus retry, got %#v", embedder.calls)
	}
}

type recordingVectorStageEmbedder struct {
	calls      [][]string
	failures   int
	embeddings [][]float32
}

func (e *recordingVectorStageEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	e.calls = append(e.calls, append([]string(nil), texts...))
	if e.failures > 0 {
		e.failures--
		return nil, errors.New("temporary failure")
	}
	if e.embeddings != nil {
		return e.embeddings, nil
	}
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = []float32{float32(len(texts)), float32(i)}
	}
	return embeddings, nil
}

type recordingVectorStageStore struct {
	collection string
	ids        []string
	embeddings [][]float32
	documents  []string
	metadatas  []map[string]any
}

func (s *recordingVectorStageStore) Upsert(_ context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error {
	s.collection = collection
	s.ids = append([]string(nil), ids...)
	s.embeddings = append([][]float32(nil), embeddings...)
	s.documents = append([]string(nil), documents...)
	s.metadatas = append([]map[string]any(nil), metadatas...)
	return nil
}
