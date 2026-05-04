package parser

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRoutesPlainTextAndUnsupported(t *testing.T) {
	dir := t.TempDir()
	txtPath := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(txtPath, []byte("\ufeffhello\n\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := Parse(txtPath, "text/plain; charset=utf-8")
	if err != nil {
		t.Fatalf("Parse plain text: %v", err)
	}
	if doc.Text != "hello\n\nworld" {
		t.Fatalf("unexpected text: %q", doc.Text)
	}

	_, err = Parse(filepath.Join(dir, "archive.zip"), "application/zip")
	if !errors.Is(err, ErrUnsupportedFormat) {
		t.Fatalf("expected ErrUnsupportedFormat, got %v", err)
	}
}

func TestParsePlainTextNormalizesInvalidUTF8(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.log")
	if err := os.WriteFile(path, []byte{'o', 'k', 0xff, '\n'}, 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := ParsePlainText(path)
	if err != nil {
		t.Fatalf("ParsePlainText: %v", err)
	}
	if strings.ContainsRune(doc.Text, '\ufffd') {
		t.Fatalf("invalid utf-8 replacement leaked into text: %q", doc.Text)
	}
	if doc.Text != "ok" {
		t.Fatalf("unexpected text: %q", doc.Text)
	}
}
