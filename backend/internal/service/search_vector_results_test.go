package service

import (
	"testing"

	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/vectordb"
)

func TestVectorChunkMapperMapsMetadataAndAppliesFilters(t *testing.T) {
	keepParentID := indexing.ParentChunkID("keep", 0)
	result := &vectordb.QueryResult{
		IDs:       []string{indexing.ChunkID("keep", 0), indexing.ChunkID("other", 0), indexing.ChunkID("weak", 0)},
		Documents: []string{"kept document text", "other document text", "weak document text"},
		Distances: []float32{0.2, 0.1, 0.8},
		Metadatas: []map[string]any{
			(indexing.ChunkMetadata{
				FileID:        "keep",
				FileName:      "Keep.md",
				Heading:       "Guide",
				ChunkIndex:    0,
				ParentChunkID: keepParentID,
			}).Map(),
			(indexing.ChunkMetadata{FileID: "other", FileName: "Other.md", ChunkIndex: 0}).Map(),
			(indexing.ChunkMetadata{FileID: "weak", FileName: "Weak.md", ChunkIndex: 0}).Map(),
		},
	}

	sources := (vectorChunkMapper{MinScore: 0.5}).Map(result, vectorChunkMappingOptions{
		FileIDs: []string{"keep", "weak"},
		TopK:    10,
	})

	if len(sources) != 1 {
		t.Fatalf("expected one mapped source after filters, got %#v", sources)
	}
	got := sources[0]
	if got.ID != indexing.ChunkID("keep", 0) ||
		got.FileID != "keep" ||
		got.FileName != "Keep.md" ||
		got.Heading != "Guide" ||
		got.ChunkIndex != 0 ||
		got.ParentID != keepParentID {
		t.Fatalf("metadata contract was not preserved: %#v", got)
	}
	if got.Score < 0.79 || got.Score > 0.81 {
		t.Fatalf("expected score from vector distance, got %.4f", got.Score)
	}
	if got.Snippet == "" {
		t.Fatal("expected snippet to be populated")
	}
}
