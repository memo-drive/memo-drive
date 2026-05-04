package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxDOCXXMLSize  = 50 * 1024 * 1024
	maxDOCXMetaSize = 1 * 1024 * 1024
)

func ParseDOCX(path string) (*ParsedDocument, error) {
	started := time.Now()
	reader, err := zip.OpenReader(path)
	if err != nil {
		log.Printf("level=error component=parser parser=docx event=open_failed file=%q err=%q", filepath.Base(path), err)
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer reader.Close()

	documentXML, err := readZipFileLimited(reader.File, "word/document.xml", maxDOCXXMLSize)
	if err != nil {
		log.Printf("level=error component=parser parser=docx event=document_xml_read_failed file=%q duration_ms=%d err=%q", filepath.Base(path), time.Since(started).Milliseconds(), err)
		return nil, err
	}

	text, err := extractDOCXText(documentXML)
	if err != nil {
		log.Printf("level=error component=parser parser=docx event=document_xml_parse_failed file=%q duration_ms=%d err=%q", filepath.Base(path), time.Since(started).Milliseconds(), err)
		return nil, err
	}

	meta := map[string]string{"format": "docx"}
	if coreXML, err := readZipFileLimited(reader.File, "docProps/core.xml", maxDOCXMetaSize); err == nil {
		for key, value := range extractDOCXCoreMeta(coreXML) {
			if value != "" {
				meta[key] = value
			}
		}
	}

	cleanText := cleanExtractedText(text)
	log.Printf("level=info component=parser parser=docx event=parse_complete file=%q chars=%d title=%q duration_ms=%d",
		filepath.Base(path), len([]rune(cleanText)), meta["title"], time.Since(started).Milliseconds())
	return &ParsedDocument{
		Text:  cleanText,
		Title: meta["title"],
		Meta:  meta,
	}, nil
}

func readZipFileLimited(files []*zip.File, name string, limit int64) ([]byte, error) {
	for _, file := range files {
		if file.Name != name {
			continue
		}
		if file.UncompressedSize64 > uint64(limit) {
			return nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()

		var buf bytes.Buffer
		n, err := io.Copy(&buf, io.LimitReader(rc, limit+1))
		if err != nil {
			return nil, err
		}
		if n > limit {
			return nil, fmt.Errorf("%s exceeds %d bytes", name, limit)
		}
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("docx missing %s", name)
}

func extractDOCXText(documentXML []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(documentXML))
	var paragraphs []string
	var paragraph strings.Builder
	var cell strings.Builder
	var rowCells []string
	inText := false
	inTable := false
	inCell := false
	skipDepth := 0

	appendText := func(text string) {
		if inCell {
			paragraph.WriteString(text)
			return
		}
		paragraph.WriteString(text)
	}

	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("parse docx xml: %w", err)
		}

		switch value := token.(type) {
		case xml.StartElement:
			local := value.Name.Local
			if skipDepth > 0 {
				skipDepth++
				continue
			}
			if local == "drawing" || local == "AlternateContent" {
				skipDepth = 1
				continue
			}
			switch local {
			case "tbl":
				inTable = true
			case "tr":
				rowCells = nil
			case "tc":
				inCell = true
				cell.Reset()
			case "p":
				paragraph.Reset()
			case "t":
				inText = true
			case "tab":
				appendText("\t")
			case "br":
				appendText("\n")
			}
		case xml.EndElement:
			local := value.Name.Local
			if skipDepth > 0 {
				skipDepth--
				continue
			}
			switch local {
			case "t":
				inText = false
			case "p":
				text := cleanExtractedText(paragraph.String())
				if text == "" {
					continue
				}
				if inTable {
					if cell.Len() > 0 {
						cell.WriteByte('\n')
					}
					cell.WriteString(text)
				} else {
					paragraphs = append(paragraphs, text)
				}
			case "tc":
				rowCells = append(rowCells, cleanExtractedText(cell.String()))
				inCell = false
				cell.Reset()
			case "tr":
				if len(rowCells) > 0 {
					paragraphs = append(paragraphs, strings.Join(rowCells, "\t"))
				}
			case "tbl":
				inTable = false
			}
		case xml.CharData:
			if inText && skipDepth == 0 {
				appendText(string(value))
			}
		}
	}

	return strings.Join(paragraphs, "\n\n"), nil
}

func extractDOCXCoreMeta(coreXML []byte) map[string]string {
	decoder := xml.NewDecoder(bytes.NewReader(coreXML))
	meta := make(map[string]string)
	var current string
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "title", "creator", "subject", "description", "keywords":
				current = value.Name.Local
			default:
				current = ""
			}
		case xml.EndElement:
			if value.Name.Local == current {
				current = ""
			}
		case xml.CharData:
			if current != "" {
				key := current
				if key == "creator" {
					key = "author"
				}
				meta[key] = strings.TrimSpace(string(value))
			}
		}
	}
	return meta
}
