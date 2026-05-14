package parser

import (
	"errors"
	"fmt"
	"log"
	"mime"
	"path/filepath"
	"strings"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported document format")
	ErrProtectedPDF      = errors.New("protected pdf")
	ErrImageTooLarge     = errors.New("image file is too large for ocr")
	ErrAudioTooLarge     = errors.New("audio file is too large for transcription")
)

type ParsedDocument struct {
	Text     string            // Extracted plain text.
	Title    string            // Optional document title.
	Sections []Section         // Optional structured sections for heading-aware splitting.
	Meta     map[string]string // Parser-specific metadata such as pages or author.
}

type Section struct {
	Heading string // Section heading when available.
	Body    string // Plain-text section body.
}

func (doc *ParsedDocument) HasContent() bool {
	if doc == nil {
		return false
	}
	if strings.TrimSpace(doc.Text) != "" {
		return true
	}
	for _, section := range doc.Sections {
		if strings.TrimSpace(section.Heading) != "" || strings.TrimSpace(section.Body) != "" {
			return true
		}
	}
	return false
}

func Parse(filePath string, mimeType string) (*ParsedDocument, error) {
	normalizedMIME := normalizeMIME(mimeType)
	ext := strings.ToLower(filepath.Ext(filePath))
	log.Printf("level=debug component=parser event=route path=%q mime_type=%q normalized_mime=%q ext=%q", filepath.Base(filePath), mimeType, normalizedMIME, ext)

	switch {
	case normalizedMIME == "application/pdf" || ext == ".pdf":
		return ParsePDF(filePath)
	case normalizedMIME == "application/vnd.openxmlformats-officedocument.wordprocessingml.document" || ext == ".docx":
		return ParseDOCX(filePath)
	case normalizedMIME == "text/markdown" || ext == ".md" || ext == ".markdown":
		return ParseMarkdown(filePath)
	case normalizedMIME == "text/plain" || isPlainTextExt(ext):
		return ParsePlainText(filePath)
	case strings.HasPrefix(normalizedMIME, "text/") && normalizedMIME != "text/html":
		return ParsePlainText(filePath)
	default:
		if normalizedMIME == "" {
			normalizedMIME = "unknown"
		}
		return nil, fmt.Errorf("%w: mime=%s ext=%s", ErrUnsupportedFormat, normalizedMIME, ext)
	}
}

func normalizeMIME(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	if parsed, _, err := mime.ParseMediaType(value); err == nil {
		return parsed
	}
	if idx := strings.IndexByte(value, ';'); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func isPlainTextExt(ext string) bool {
	switch ext {
	case ".txt", ".csv", ".log", ".json", ".xml", ".yaml", ".yml":
		return true
	default:
		return false
	}
}
