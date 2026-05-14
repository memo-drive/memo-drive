package parser

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ParsePlainText reads a plain text file, strips BOM if present, and returns
// a ParsedDocument with normalized whitespace.
func ParsePlainText(path string) (*ParsedDocument, error) {
	started := time.Now()
	body, err := os.ReadFile(path)
	if err != nil {
		log.Printf("level=error component=parser parser=plaintext event=read_failed file=%q err=%q", filepath.Base(path), err)
		return nil, err
	}
	text := strings.TrimPrefix(string(body), "\ufeff")
	cleanText := cleanExtractedText(strings.ToValidUTF8(text, ""))
	log.Printf("level=info component=parser parser=plaintext event=parse_complete file=%q bytes=%d chars=%d duration_ms=%d",
		filepath.Base(path), len(body), len([]rune(cleanText)), time.Since(started).Milliseconds())
	return &ParsedDocument{
		Text: cleanText,
		Meta: map[string]string{
			"format": "plaintext",
		},
	}, nil
}
