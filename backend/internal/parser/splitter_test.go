package parser

import (
	"strings"
	"testing"
)

func TestSplitDocumentUsesSectionsAndBreakpoints(t *testing.T) {
	doc := &ParsedDocument{
		Sections: []Section{
			{
				Heading: "章节一",
				Body:    "第一句比较短。第二句包含更多中文内容，需要被切开。第三句结束。",
			},
		},
	}

	chunks := SplitDocument(doc, SplitOptions{ChunkSize: 18, Overlap: 4})
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0].Heading != "章节一" || chunks[0].Index != 0 || chunks[1].Index != 1 {
		t.Fatalf("unexpected chunk metadata: %#v", chunks)
	}
	if !strings.HasSuffix(chunks[0].Text, "。") {
		t.Fatalf("expected first chunk to break at punctuation: %q", chunks[0].Text)
	}
}

func TestSplitTextKeepsOverlap(t *testing.T) {
	chunks := SplitText("abcdefghijklmnopqrstuvwxyz", 10)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	if chunks[0] != "abcdefghij" {
		t.Fatalf("unexpected first chunk: %q", chunks[0])
	}
	if !strings.HasPrefix(chunks[1], "ij") {
		t.Fatalf("expected overlap prefix, got %q", chunks[1])
	}
}
