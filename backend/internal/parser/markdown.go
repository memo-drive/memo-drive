package parser

import (
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type markdownHeading struct {
	Level int
	Title string
	Line  int
}

var (
	headingPattern     = regexp.MustCompile(`^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$`)
	linkPattern        = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]+\)`)
	htmlTagPattern     = regexp.MustCompile(`<[^>]+>`)
	orderedListPattern = regexp.MustCompile(`^\s*\d+\.\s+`)
	bulletListPattern  = regexp.MustCompile(`^\s*[-+*]\s+`)
)

// ParseMarkdown reads a Markdown file and extracts structured sections based on
// ATX heading markers (#). Inline formatting, images, links, and HTML tags are stripped.
func ParseMarkdown(path string) (*ParsedDocument, error) {
	started := time.Now()
	body, err := os.ReadFile(path)
	if err != nil {
		log.Printf("level=error component=parser parser=markdown event=read_failed file=%q err=%q", filepath.Base(path), err)
		return nil, err
	}

	raw := strings.TrimPrefix(string(body), "\ufeff")
	raw = strings.ToValidUTF8(raw, "")
	text := stripMarkdown(raw)
	sections := markdownSections(raw)

	title := ""
	for _, section := range sections {
		if section.Heading != "" {
			title = section.Heading
			break
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}

	cleanText := cleanExtractedText(text)
	log.Printf("level=info component=parser parser=markdown event=parse_complete file=%q chars=%d sections=%d title=%q duration_ms=%d",
		filepath.Base(path), len([]rune(cleanText)), len(sections), title, time.Since(started).Milliseconds())
	return &ParsedDocument{
		Text:     cleanText,
		Title:    title,
		Sections: sections,
		Meta: map[string]string{
			"format": "markdown",
		},
	}, nil
}

func markdownSections(raw string) []Section {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	headings := make([]markdownHeading, 0)
	for i, line := range lines {
		matches := headingPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		headings = append(headings, markdownHeading{
			Level: len(matches[1]),
			Title: cleanInlineMarkdown(matches[2]),
			Line:  i,
		})
	}

	sections := make([]Section, 0, len(headings)+1)
	if len(headings) == 0 {
		text := cleanExtractedText(stripMarkdown(raw))
		if text != "" {
			sections = append(sections, Section{Body: text})
		}
		return sections
	}

	if preamble := cleanExtractedText(stripMarkdown(strings.Join(lines[:headings[0].Line], "\n"))); preamble != "" {
		sections = append(sections, Section{Body: preamble})
	}

	for i := range headings {
		endLine := len(lines)
		for j := i + 1; j < len(headings); j++ {
			if headings[j].Level <= headings[i].Level {
				endLine = headings[j].Line
				break
			}
		}
		body := ""
		if headings[i].Line+1 < endLine {
			body = cleanExtractedText(stripMarkdown(strings.Join(lines[headings[i].Line+1:endLine], "\n")))
		}
		if body == "" {
			continue
		}
		sections = append(sections, Section{
			Heading: headings[i].Title,
			Body:    body,
		})
	}

	return sections
}

func stripMarkdown(raw string) string {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	raw = strings.ReplaceAll(raw, "\r", "\n")
	lines := strings.Split(raw, "\n")
	var builder strings.Builder
	inFence := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			inFence = !inFence
			continue
		}
		if inFence {
			builder.WriteString(line)
			builder.WriteByte('\n')
			continue
		}

		if matches := headingPattern.FindStringSubmatch(line); len(matches) == 3 {
			line = matches[2]
		}
		line = strings.TrimPrefix(strings.TrimSpace(line), "> ")
		line = orderedListPattern.ReplaceAllString(line, "")
		line = bulletListPattern.ReplaceAllString(line, "")
		line = cleanInlineMarkdown(line)
		builder.WriteString(line)
		builder.WriteByte('\n')
	}

	return builder.String()
}

func cleanInlineMarkdown(text string) string {
	text = linkPattern.ReplaceAllString(text, "$1")
	text = htmlTagPattern.ReplaceAllString(text, "")
	replacer := strings.NewReplacer(
		"`", "",
		"**", "",
		"__", "",
		"~~", "",
	)
	text = replacer.Replace(text)
	text = strings.ReplaceAll(text, "*", "")
	text = strings.ReplaceAll(text, "_", "")
	return strings.TrimSpace(text)
}
