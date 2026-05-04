package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMarkdownExtractsSectionsAndPlainText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.md")
	input := `# MemoDrive **Guide**

Intro with [a link](https://example.com) and ` + "`inline code`" + `.

## Install

- Run **make dev**

` + "```go" + `
fmt.Println("kept")
` + "```" + `
`
	if err := os.WriteFile(path, []byte(input), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := ParseMarkdown(path)
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if doc.Title != "MemoDrive Guide" {
		t.Fatalf("unexpected title: %q", doc.Title)
	}
	if len(doc.Sections) < 2 {
		t.Fatalf("expected at least 2 sections, got %d", len(doc.Sections))
	}
	if doc.Sections[0].Heading != "MemoDrive Guide" {
		t.Fatalf("unexpected first heading: %q", doc.Sections[0].Heading)
	}
	if strings.Contains(doc.Text, "**") || strings.Contains(doc.Text, "](https://") {
		t.Fatalf("markdown syntax was not stripped: %q", doc.Text)
	}
	for _, want := range []string{"Intro with a link", "inline code", `fmt.Println("kept")`} {
		if !strings.Contains(doc.Text, want) {
			t.Fatalf("expected %q in text: %q", want, doc.Text)
		}
	}
}
