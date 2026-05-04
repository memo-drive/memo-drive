package parser

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	pageNumberLinePattern = regexp.MustCompile(`^\d{1,5}$`)
	multiNewlinePattern   = regexp.MustCompile(`\n{3,}`)
	multiSpacePattern     = regexp.MustCompile(` {2,}`)
)

func cleanExtractedText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	var builder strings.Builder
	builder.Grow(len(text))
	for _, r := range text {
		switch {
		case r == '\n':
			builder.WriteRune('\n')
		case r == '\t':
			builder.WriteRune('\t')
		case unicode.IsSpace(r):
			builder.WriteByte(' ')
		default:
			builder.WriteRune(r)
		}
	}

	lines := strings.Split(builder.String(), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(multiSpacePattern.ReplaceAllString(line, " "))
		if pageNumberLinePattern.MatchString(line) {
			continue
		}
		cleaned = append(cleaned, line)
	}

	text = strings.Join(cleaned, "\n")
	text = multiNewlinePattern.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}

func ensureMeta(meta map[string]string) map[string]string {
	if meta != nil {
		return meta
	}
	return make(map[string]string)
}
