package parser

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDOCXExtractsParagraphsTablesAndMeta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample.docx")
	if err := writeTestDOCX(path); err != nil {
		t.Fatal(err)
	}

	doc, err := ParseDOCX(path)
	if err != nil {
		t.Fatalf("ParseDOCX: %v", err)
	}
	if doc.Title != "Sample Title" {
		t.Fatalf("unexpected title: %q", doc.Title)
	}
	for _, want := range []string{"First paragraph", "Second\tline", "A1\tB1"} {
		if !strings.Contains(doc.Text, want) {
			t.Fatalf("expected %q in text: %q", want, doc.Text)
		}
	}
	if doc.Meta["author"] != "Ada" {
		t.Fatalf("unexpected author: %q", doc.Meta["author"])
	}
}

func writeTestDOCX(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	zw := zip.NewWriter(file)
	defer zw.Close()

	document := `<?xml version="1.0" encoding="UTF-8"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>First paragraph</w:t></w:r></w:p>
    <w:p><w:r><w:t>Second</w:t><w:tab/><w:t>line</w:t></w:r></w:p>
    <w:tbl>
      <w:tr>
        <w:tc><w:p><w:r><w:t>A1</w:t></w:r></w:p></w:tc>
        <w:tc><w:p><w:r><w:t>B1</w:t></w:r></w:p></w:tc>
      </w:tr>
    </w:tbl>
  </w:body>
</w:document>`
	core := `<?xml version="1.0" encoding="UTF-8"?>
<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">
  <dc:title>Sample Title</dc:title>
  <dc:creator>Ada</dc:creator>
</cp:coreProperties>`

	if err := writeZipEntry(zw, "word/document.xml", document); err != nil {
		return err
	}
	return writeZipEntry(zw, "docProps/core.xml", core)
}

func writeZipEntry(zw *zip.Writer, name, body string) error {
	writer, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = writer.Write([]byte(body))
	return err
}
