package parser

import (
	"context"
	"errors"
	"os/exec"
	"testing"

	"github.com/memodrive/backend/internal/config"
)

func TestParseImageOCRHappyPath(t *testing.T) {
	runner := &OCRRunner{
		ready:        true,
		langs:        "eng+chi_sim",
		minTextRunes: 3,
		runFunc: func(ctx context.Context, imagePath string) (string, error) {
			return "hello\n世\n世界", nil
		},
	}

	doc, err := ParseImageOCR(context.Background(), runner, "/tmp/sample.png")
	if err != nil {
		t.Fatalf("ParseImageOCR returned error: %v", err)
	}
	if doc.Text != "hello\n世界" {
		t.Fatalf("unexpected OCR text %q", doc.Text)
	}
	if doc.Meta["source"] != "image_ocr" || doc.Meta["lang"] != "eng+chi_sim" {
		t.Fatalf("unexpected metadata %#v", doc.Meta)
	}
	if len(doc.Sections) != 1 || doc.Sections[0].Heading != "OCR" {
		t.Fatalf("unexpected sections %#v", doc.Sections)
	}
}

func TestParseImageOCRUnavailableReturnsEmptyDocument(t *testing.T) {
	doc, err := ParseImageOCR(context.Background(), &OCRRunner{reason: "disabled"}, "/tmp/sample.png")
	if err != nil {
		t.Fatalf("ParseImageOCR returned error: %v", err)
	}
	if doc.Text != "" {
		t.Fatalf("expected empty document, got %q", doc.Text)
	}
}

func TestParseImageOCRShortTextSkips(t *testing.T) {
	runner := &OCRRunner{
		ready:        true,
		minTextRunes: 8,
		runFunc: func(ctx context.Context, imagePath string) (string, error) {
			return "abc", nil
		},
	}
	doc, err := ParseImageOCR(context.Background(), runner, "/tmp/sample.png")
	if err != nil {
		t.Fatalf("ParseImageOCR returned error: %v", err)
	}
	if doc.Text != "" || doc.Meta["skipped"] == "" {
		t.Fatalf("expected skipped empty document, got text=%q meta=%#v", doc.Text, doc.Meta)
	}
}

func TestNewOCRRunnerUnavailable(t *testing.T) {
	original := execLookPath
	execLookPath = func(file string) (string, error) {
		return "", exec.ErrNotFound
	}
	t.Cleanup(func() { execLookPath = original })

	runner := NewOCRRunner(config.OCRConfig{Enabled: true, Bin: "missing-tesseract"})
	if runner.Available() {
		t.Fatal("expected runner to be unavailable")
	}
}

func TestOCRRunnerRunReturnsUnderlyingErrors(t *testing.T) {
	expected := errors.New("ocr failed")
	runner := &OCRRunner{
		ready: true,
		runFunc: func(ctx context.Context, imagePath string) (string, error) {
			return "", expected
		},
	}
	if _, err := runner.Run(context.Background(), "/tmp/sample.png"); !errors.Is(err, expected) {
		t.Fatalf("expected underlying error, got %v", err)
	}
}
