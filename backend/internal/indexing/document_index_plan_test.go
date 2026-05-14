package indexing

import (
	"strings"
	"testing"

	"github.com/memodrive/backend/internal/parser"
)

func TestBuildDocumentIndexPlanCreatesVectorAndChunkRecords(t *testing.T) {
	doc := &parser.ParsedDocument{
		Sections: []parser.Section{{
			Heading: "API",
			Body:    strings.Repeat("unique token in api docs. ", 12),
		}},
		Meta: map[string]string{"source": "markdown"},
	}

	plan := BuildDocumentIndexPlan(DocumentRef{ID: "file-1", Name: "Guide.md"}, doc, DocumentIndexOptions{
		ParentChunkSize: 160,
		ChildChunkSize:  60,
		ChunkOverlap:    12,
	})

	if plan.ChildCount() == 0 {
		t.Fatal("expected child chunks for indexing")
	}
	if len(plan.VectorIDs) != plan.ChildCount() || len(plan.VectorTexts) != plan.ChildCount() || len(plan.VectorMetadatas) != plan.ChildCount() {
		t.Fatalf("expected vector records to match child chunks, got ids=%d texts=%d metadata=%d children=%d",
			len(plan.VectorIDs), len(plan.VectorTexts), len(plan.VectorMetadatas), plan.ChildCount())
	}
	if plan.VectorIDs[0] != ChunkID("file-1", 0) {
		t.Fatalf("unexpected first vector id: %q", plan.VectorIDs[0])
	}
	if !strings.HasPrefix(plan.VectorTexts[0], "## API\n") {
		t.Fatalf("expected vector text to include heading context, got %q", plan.VectorTexts[0])
	}

	metadata := ChunkMetadataFromMap(plan.VectorMetadatas[0])
	if metadata.FileID != "file-1" ||
		metadata.FileName != "Guide.md" ||
		metadata.Heading != "API" ||
		metadata.ChunkIndex != 0 ||
		metadata.Source != "markdown" ||
		metadata.ParentChunkID != ParentChunkID("file-1", 0) {
		t.Fatalf("unexpected vector metadata: %#v", metadata)
	}

	if len(plan.ChunkRecords) <= plan.ChildCount() {
		t.Fatalf("expected parent and child chunk records, got %#v", plan.ChunkRecords)
	}
	if !plan.ChunkRecords[0].IsParent || plan.ChunkRecords[0].ID != ParentChunkID("file-1", 0) {
		t.Fatalf("expected first chunk record to be parent context, got %#v", plan.ChunkRecords[0])
	}
	child := firstChildRecord(t, plan)
	if child.ID != ChunkID("file-1", 0) || child.ParentChunkID != ParentChunkID("file-1", 0) {
		t.Fatalf("expected child record to link to parent context, got %#v", child)
	}
}

func firstChildRecord(t *testing.T, plan DocumentIndexPlan) ChunkRecord {
	t.Helper()
	for _, record := range plan.ChunkRecords {
		if !record.IsParent {
			return record
		}
	}
	t.Fatal("missing child chunk record")
	return ChunkRecord{}
}
