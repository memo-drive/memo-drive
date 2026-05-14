package parser

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	pdf "github.com/ledongthuc/pdf"
)

const maxPDFPages = 500

// ParsePDF extracts plain text from a PDF file page by page, up to maxPDFPages.
func ParsePDF(path string) (*ParsedDocument, error) {
	started := time.Now()
	file, err := os.Open(path)
	if err != nil {
		log.Printf("level=error component=parser parser=pdf event=open_failed file=%q err=%q", filepath.Base(path), err)
		return nil, err
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		log.Printf("level=error component=parser parser=pdf event=stat_failed file=%q err=%q", filepath.Base(path), err)
		return nil, err
	}

	reader, err := pdf.NewReader(file, stat.Size())
	if err != nil {
		if errors.Is(err, pdf.ErrInvalidPassword) || strings.Contains(strings.ToLower(err.Error()), "encrypted") {
			log.Printf("level=warn component=parser parser=pdf event=protected file=%q size=%d duration_ms=%d err=%q", filepath.Base(path), stat.Size(), time.Since(started).Milliseconds(), err)
			return nil, fmt.Errorf("%w: %v", ErrProtectedPDF, err)
		}
		log.Printf("level=error component=parser parser=pdf event=open_reader_failed file=%q size=%d duration_ms=%d err=%q", filepath.Base(path), stat.Size(), time.Since(started).Milliseconds(), err)
		return nil, fmt.Errorf("parse pdf: %w", err)
	}

	totalPages := reader.NumPage()
	pageLimit := totalPages
	meta := map[string]string{
		"format": "pdf",
		"pages":  strconv.Itoa(totalPages),
	}
	if pageLimit > maxPDFPages {
		pageLimit = maxPDFPages
		meta["truncated"] = "true"
		meta["truncated_at_page"] = strconv.Itoa(maxPDFPages)
		log.Printf("level=warn component=parser parser=pdf event=truncated file=%q truncated_at_page=%d total_pages=%d", filepath.Base(path), maxPDFPages, totalPages)
	}

	var builder strings.Builder
	fonts := make(map[string]*pdf.Font)
	for pageIndex := 1; pageIndex <= pageLimit; pageIndex++ {
		page := reader.Page(pageIndex)
		for _, name := range page.Fonts() {
			if _, ok := fonts[name]; !ok {
				font := page.Font(name)
				fonts[name] = &font
			}
		}
		text, err := page.GetPlainText(fonts)
		if err != nil {
			log.Printf("level=error component=parser parser=pdf event=page_parse_failed file=%q page=%d duration_ms=%d err=%q", filepath.Base(path), pageIndex, time.Since(started).Milliseconds(), err)
			return nil, fmt.Errorf("parse pdf page %d: %w", pageIndex, err)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		builder.WriteString(text)
		if !strings.HasSuffix(text, "\n") {
			builder.WriteByte('\n')
		}
	}

	text := cleanExtractedText(builder.String())
	log.Printf("level=info component=parser parser=pdf event=parse_complete file=%q pages=%d parsed_pages=%d chars=%d truncated=%t duration_ms=%d",
		filepath.Base(path), totalPages, pageLimit, len([]rune(text)), meta["truncated"] == "true", time.Since(started).Milliseconds())
	return &ParsedDocument{
		Text: text,
		Meta: meta,
	}, nil
}
