package indexing

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestChunkMetadataRoundTripsVectorMetadata(t *testing.T) {
	parentID := ParentChunkID("file-1", 2)
	metadata := ChunkMetadata{
		FileID:        "file-1",
		FileName:      "Guide.md",
		Heading:       "Intro",
		ChunkIndex:    7,
		Source:        "markdown",
		ParentChunkID: parentID,
	}

	if got := ChunkMetadataFromMap(metadata.Map()); !reflect.DeepEqual(got, metadata) {
		t.Fatalf("metadata did not round trip: got %#v want %#v", got, metadata)
	}
}

func TestChunkMetadataParsesChromaNumberShapes(t *testing.T) {
	got := ChunkMetadataFromMap(map[string]any{
		MetadataFileID:        "file-1",
		MetadataFileName:      "Guide.md",
		MetadataChunkIndex:    json.Number("3"),
		MetadataParentChunkID: ParentChunkID("file-1", 0),
	})

	if got.ChunkIndex != 3 {
		t.Fatalf("expected json.Number chunk index to parse, got %d", got.ChunkIndex)
	}
}

func TestChunkIDsUseStableVectorIndexFormat(t *testing.T) {
	if got := ChunkID("file-1", 3); got != "file-1#3" {
		t.Fatalf("unexpected child chunk id: %q", got)
	}
	if got := ParentChunkID("file-1", 2); got != "file-1#parent-2" {
		t.Fatalf("unexpected parent chunk id: %q", got)
	}
	if got := ChunkIDs("file-1", 3); !reflect.DeepEqual(got, []string{"file-1#0", "file-1#1", "file-1#2"}) {
		t.Fatalf("unexpected chunk ids: %#v", got)
	}
}
