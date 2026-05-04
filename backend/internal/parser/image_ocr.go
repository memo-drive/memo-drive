package parser

import (
	"context"
	"log"
	"path/filepath"
	"time"
)

func ParseImageOCR(ctx context.Context, runner *OCRRunner, absPath string) (*ParsedDocument, error) {
	started := time.Now()
	if runner == nil || !runner.Available() {
		log.Printf("level=warn component=parser parser=image_ocr event=skipped file=%q reason=ocr_unavailable", filepath.Base(absPath))
		return &ParsedDocument{Meta: map[string]string{"source": "image_ocr", "skipped": "ocr_unavailable"}}, nil
	}
	text, err := runner.Run(ctx, absPath)
	if err != nil {
		if isTooLargeErr(err) {
			return &ParsedDocument{Meta: map[string]string{"source": "image_ocr", "skipped": err.Error()}}, nil
		}
		return nil, err
	}
	if text == "" {
		log.Printf("level=info component=parser parser=image_ocr event=empty file=%q duration_ms=%d", filepath.Base(absPath), time.Since(started).Milliseconds())
		return &ParsedDocument{Meta: map[string]string{"source": "image_ocr", "skipped": "empty_text"}}, nil
	}
	doc := &ParsedDocument{
		Text:  text,
		Title: filepath.Base(absPath),
		Sections: []Section{{
			Heading: "OCR",
			Body:    text,
		}},
		Meta: map[string]string{
			"source": "image_ocr",
			"lang":   runner.Langs(),
		},
	}
	log.Printf("level=info component=parser parser=image_ocr event=complete file=%q runes=%d duration_ms=%d",
		filepath.Base(absPath), len([]rune(text)), time.Since(started).Milliseconds())
	return doc, nil
}
