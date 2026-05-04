package store

import (
	"context"
	"errors"
	"testing"

	"github.com/memodrive/backend/internal/model"
)

func TestChunkStoreUpsertSearchParentAndDelete(t *testing.T) {
	db := newSearchTestStore(t)
	file := &model.File{ID: "doc-a", Name: "Doc.md", Path: "/", StoragePath: "doc.md", Size: 100, MimeType: "text/markdown", Status: model.FileStatusReady}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	rows := []ChunkRow{
		{ID: "doc-a#parent-0", FileID: file.ID, FileName: file.Name, Heading: "Intro", ChunkIndex: 0, Text: "parent context with exact-token", IsParent: true},
		{ID: "doc-a#0", FileID: file.ID, FileName: file.Name, Heading: "Intro", ChunkIndex: 0, Text: "child exact-token", ParentChunkID: "doc-a#parent-0"},
	}
	if err := db.UpsertChunks(context.Background(), rows); err != nil {
		t.Fatalf("upsert chunks: %v", err)
	}

	text, err := db.GetChunkText(context.Background(), "doc-a#parent-0")
	if err != nil {
		t.Fatalf("get parent text: %v", err)
	}
	if text != "parent context with exact-token" {
		t.Fatalf("unexpected parent text: %q", text)
	}

	results, err := db.SearchChunksBM25(context.Background(), "exact-token", nil, 10)
	if err != nil {
		t.Fatalf("search chunks: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected child result only, got %#v", results)
	}
	if results[0].ID != "doc-a#0" || results[0].ParentID != "doc-a#parent-0" {
		t.Fatalf("unexpected search result: %#v", results[0])
	}

	if err := db.DeleteChunksByFileID(context.Background(), file.ID); err != nil {
		t.Fatalf("delete chunks: %v", err)
	}
	if _, err := db.GetChunkText(context.Background(), "doc-a#parent-0"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected parent text to be deleted, got %v", err)
	}
}
