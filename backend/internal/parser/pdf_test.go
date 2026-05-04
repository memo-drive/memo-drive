package parser

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePDFExtractsPlainText(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(path, buildTestPDF("Hello PDF"), 0o644); err != nil {
		t.Fatal(err)
	}

	doc, err := ParsePDF(path)
	if err != nil {
		t.Fatalf("ParsePDF: %v", err)
	}
	if !strings.Contains(doc.Text, "Hello PDF") {
		t.Fatalf("expected extracted text, got %q", doc.Text)
	}
	if doc.Meta["pages"] != "1" {
		t.Fatalf("unexpected pages meta: %q", doc.Meta["pages"])
	}
}

func buildTestPDF(text string) []byte {
	stream := fmt.Sprintf("BT\n/F1 24 Tf\n72 720 Td\n(%s) Tj\nET\n", text)
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", len(stream), stream),
	}

	var builder strings.Builder
	builder.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects))
	for i, object := range objects {
		offsets = append(offsets, builder.Len())
		fmt.Fprintf(&builder, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xrefOffset := builder.Len()
	fmt.Fprintf(&builder, "xref\n0 %d\n", len(objects)+1)
	builder.WriteString("0000000000 65535 f \n")
	for _, offset := range offsets {
		fmt.Fprintf(&builder, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&builder, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)
	return []byte(builder.String())
}
