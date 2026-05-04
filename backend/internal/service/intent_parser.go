package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/memodrive/backend/internal/llm"
)

const (
	defaultIntentTimezone = "Asia/Shanghai"
)

type SearchIntentOptions struct {
	Now         time.Time
	Timezone    string
	LLMFallback bool
}

var extMimeMap = map[string]string{
	"pdf":  "application/pdf",
	"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml",
	"xls":  "application/vnd.ms-excel",
	"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml",
	"doc":  "application/msword",
	"pptx": "application/vnd.openxmlformats-officedocument.presentationml",
	"ppt":  "application/vnd.ms-powerpoint",
	"md":   "text/markdown",
	"txt":  "text/plain",
	"csv":  "text/csv",
	"log":  "text/plain",
	"json": "application/json",
	"xml":  "application/xml",
	"yaml": "application/x-yaml",
	"yml":  "application/x-yaml",
}

var cnTypeMimeMap = map[string]string{
	"图片":   "image/",
	"照片":   "image/",
	"相片":   "image/",
	"视频":   "video/",
	"音频":   "audio/",
	"录音":   "audio/",
	"文档":   "application/",
	"表格":   "application/vnd.openxmlformats-officedocument.spreadsheetml",
	"电子表格": "application/vnd.openxmlformats-officedocument.spreadsheetml",
	"演示文稿": "application/vnd.openxmlformats-officedocument.presentationml",
}

var englishTypeMimeMap = map[string]string{
	"image":       "image/",
	"images":      "image/",
	"photo":       "image/",
	"photos":      "image/",
	"video":       "video/",
	"videos":      "video/",
	"audio":       "audio/",
	"document":    "application/",
	"documents":   "application/",
	"spreadsheet": "application/vnd.openxmlformats-officedocument.spreadsheetml",
	"slides":      "application/vnd.openxmlformats-officedocument.presentationml",
}

var (
	yearMonthCNPattern     = regexp.MustCompile(`([12][0-9]{3})年([0-9]{1,2})月`)
	yearMonthDashPattern   = regexp.MustCompile(`\b([12][0-9]{3})[-/]([0-9]{1,2})\b`)
	relativeMonthPattern   = regexp.MustCompile(`(?i)(最近|近)?\s*([0-9]+|一|二|两|三|四|五|六|七|八|九|十|半)\s*个?月(前|内)?`)
	relativeDayPattern     = regexp.MustCompile(`(?i)(最近|近)?\s*([0-9]+|一|二|两|三|四|五|六|七|八|九|十)\s*(天|日)(前|内)?`)
	intentJSONBlockPattern = regexp.MustCompile("(?s)\\{.*\\}")
)

func ParseSearchIntent(ctx context.Context, query string, provider llm.Provider) SearchIntent {
	return ParseSearchIntentWithOptions(ctx, query, provider, SearchIntentOptions{
		Now:         time.Now(),
		Timezone:    defaultIntentTimezone,
		LLMFallback: true,
	})
}

func ParseSearchIntentWithOptions(ctx context.Context, query string, provider llm.Provider, opts SearchIntentOptions) SearchIntent {
	original := strings.TrimSpace(query)
	intent := SearchIntent{Original: original, TextQuery: original}
	if original == "" {
		return intent
	}

	loc := intentLocation(opts.Timezone)
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	now = now.In(loc)

	working := original
	var removed bool
	working, removed = extractTimeByRules(&intent, working, now, loc)
	typeRemoved := extractTypeByRules(&intent, &working)
	removed = removed || typeRemoved
	if removed {
		intent.TextQuery = cleanupIntentText(working)
	}
	intent.Extensions = uniqueStrings(intent.Extensions)
	intent.MimeTypes = uniqueStrings(intent.MimeTypes)

	if !intent.HasFilters() && opts.LLMFallback && provider != nil && looksLikeStructuredIntent(original) {
		if parsed, ok := llmParseIntent(ctx, original, provider, now); ok {
			intent = mergeLLMIntent(intent, parsed)
		}
	}
	log.Printf("level=info component=search event=intent_parse original_chars=%d text_query_chars=%d mime_types=%d extensions=%d date_from=%q date_to=%q",
		len([]rune(original)), len([]rune(intent.TextQuery)), len(intent.MimeTypes), len(intent.Extensions), timePtrString(intent.DateFrom), timePtrString(intent.DateTo))
	return intent
}

func extractTimeByRules(intent *SearchIntent, query string, now time.Time, loc *time.Location) (string, bool) {
	type timeRule struct {
		phrase string
		from   time.Time
		to     time.Time
	}
	rules := []timeRule{
		{"今天", startOfDay(now), endOfDay(now)},
		{"今日", startOfDay(now), endOfDay(now)},
		{"昨天", startOfDay(now.AddDate(0, 0, -1)), endOfDay(now.AddDate(0, 0, -1))},
		{"昨日", startOfDay(now.AddDate(0, 0, -1)), endOfDay(now.AddDate(0, 0, -1))},
		{"前天", startOfDay(now.AddDate(0, 0, -2)), endOfDay(now.AddDate(0, 0, -2))},
		{"最近一周", now.AddDate(0, 0, -7), now},
		{"近一周", now.AddDate(0, 0, -7), now},
		{"最近一星期", now.AddDate(0, 0, -7), now},
		{"最近一月", now.AddDate(0, -1, 0), now},
		{"最近一个月", now.AddDate(0, -1, 0), now},
		{"近一个月", now.AddDate(0, -1, 0), now},
		{"三个月前", startOfDay(now.AddDate(0, -3, 0)), now},
		{"最近三个月", now.AddDate(0, -3, 0), now},
		{"最近半年", now.AddDate(0, -6, 0), now},
		{"半年前", startOfDay(now.AddDate(0, -6, 0)), now},
		{"最近", now.AddDate(0, 0, -7), now},
		{"本周", startOfWeek(now), now},
		{"这周", startOfWeek(now), now},
		{"上周", startOfWeek(now).AddDate(0, 0, -7), endOfDay(startOfWeek(now).AddDate(0, 0, -1))},
		{"本月", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc), now},
		{"这个月", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc), now},
		{"上个月", time.Date(now.Year(), now.Month()-1, 1, 0, 0, 0, 0, loc), endOfDay(time.Date(now.Year(), now.Month(), 0, 0, 0, 0, 0, loc))},
		{"去年", time.Date(now.Year()-1, 1, 1, 0, 0, 0, 0, loc), time.Date(now.Year()-1, 12, 31, 23, 59, 59, int(time.Second-time.Nanosecond), loc)},
	}
	removed := false
	for _, rule := range rules {
		if strings.Contains(query, rule.phrase) {
			intent.DateFrom = utcPtr(rule.from)
			intent.DateTo = utcPtr(rule.to)
			query = strings.ReplaceAll(query, rule.phrase, " ")
			removed = true
			break
		}
	}

	if !removed {
		if match := yearMonthCNPattern.FindStringSubmatch(query); len(match) == 3 {
			if from, to, ok := monthRange(match[1], match[2], loc); ok {
				intent.DateFrom = utcPtr(from)
				intent.DateTo = utcPtr(to)
				query = strings.Replace(query, match[0], " ", 1)
				removed = true
			}
		} else if match := yearMonthDashPattern.FindStringSubmatch(query); len(match) == 3 {
			if from, to, ok := monthRange(match[1], match[2], loc); ok {
				intent.DateFrom = utcPtr(from)
				intent.DateTo = utcPtr(to)
				query = strings.Replace(query, match[0], " ", 1)
				removed = true
			}
		}
	}

	if !removed {
		if match := relativeMonthPattern.FindStringSubmatch(query); len(match) == 4 {
			if months := parseSmallNumber(match[2]); months > 0 {
				intent.DateFrom = utcPtr(startOfDay(now.AddDate(0, -months, 0)))
				intent.DateTo = utcPtr(now)
				query = strings.Replace(query, match[0], " ", 1)
				removed = true
			}
		}
	}

	if !removed {
		if match := relativeDayPattern.FindStringSubmatch(query); len(match) == 5 {
			if days := parseSmallNumber(match[2]); days > 0 {
				intent.DateFrom = utcPtr(now.AddDate(0, 0, -days))
				intent.DateTo = utcPtr(now)
				query = strings.Replace(query, match[0], " ", 1)
				removed = true
			}
		}
	}

	return query, removed
}

func extractTypeByRules(intent *SearchIntent, query *string) bool {
	removed := false
	lower := strings.ToLower(*query)
	for ext, mime := range extMimeMap {
		if !containsExtensionIntent(lower, ext) {
			continue
		}
		intent.Extensions = append(intent.Extensions, ext)
		intent.MimeTypes = append(intent.MimeTypes, mime)
		*query = removeExtensionIntent(*query, ext)
		lower = strings.ToLower(*query)
		removed = true
	}
	for token, mime := range cnTypeMimeMap {
		if strings.Contains(*query, token) {
			intent.MimeTypes = append(intent.MimeTypes, mime)
			*query = strings.ReplaceAll(*query, token, " ")
			removed = true
		}
	}
	fields := strings.Fields(lower)
	for _, field := range fields {
		token := strings.Trim(field, " ,.;:!?()[]{}")
		mime, ok := englishTypeMimeMap[token]
		if !ok {
			continue
		}
		intent.MimeTypes = append(intent.MimeTypes, mime)
		*query = removeWordToken(*query, token)
		removed = true
	}
	return removed
}

func llmParseIntent(ctx context.Context, query string, provider llm.Provider, now time.Time) (SearchIntent, bool) {
	prompt := fmt.Sprintf(`从用户的搜索查询中提取结构化信息，以 JSON 格式输出。

规则：
- text_query: 去掉时间和文件类型后的纯文本搜索关键词（如果用户只有过滤条件没有关键词，设为空字符串）
- extensions: 文件扩展名数组（如 ["pdf", "xlsx"]），不含点号
- date_from: ISO 8601 格式的起始时间（含），基于当前时间 %s
- date_to: ISO 8601 格式的截止时间（含），基于当前时间 %s
- 不确定的字段使用空字符串、空数组或 null，不要编造。

用户查询: %s`, now.Format(time.RFC3339), now.Format(time.RFC3339), query)
	result, err := provider.Complete(ctx, []llm.Message{{Role: "user", Content: prompt}})
	if err != nil {
		log.Printf("level=warn component=search event=intent_llm_failed err=%q", err)
		return SearchIntent{}, false
	}
	block := intentJSONBlockPattern.FindString(result)
	if block == "" {
		log.Printf("level=warn component=search event=intent_llm_invalid reason=no_json")
		return SearchIntent{}, false
	}
	var raw struct {
		TextQuery  string   `json:"text_query"`
		Extensions []string `json:"extensions"`
		DateFrom   *string  `json:"date_from"`
		DateTo     *string  `json:"date_to"`
	}
	if err := json.Unmarshal([]byte(block), &raw); err != nil {
		log.Printf("level=warn component=search event=intent_llm_invalid err=%q", err)
		return SearchIntent{}, false
	}
	intent := SearchIntent{Original: query, TextQuery: strings.TrimSpace(raw.TextQuery)}
	for _, ext := range raw.Extensions {
		ext = normalizeExtension(ext)
		if ext == "" {
			continue
		}
		intent.Extensions = append(intent.Extensions, ext)
		if mime := extMimeMap[ext]; mime != "" {
			intent.MimeTypes = append(intent.MimeTypes, mime)
		}
	}
	intent.DateFrom = parseOptionalTime(raw.DateFrom)
	intent.DateTo = parseOptionalTime(raw.DateTo)
	intent.Extensions = uniqueStrings(intent.Extensions)
	intent.MimeTypes = uniqueStrings(intent.MimeTypes)
	return intent, intent.HasFilters()
}

func mergeLLMIntent(base, parsed SearchIntent) SearchIntent {
	if strings.TrimSpace(parsed.TextQuery) != "" {
		base.TextQuery = strings.TrimSpace(parsed.TextQuery)
	}
	base.Extensions = uniqueStrings(append(base.Extensions, parsed.Extensions...))
	base.MimeTypes = uniqueStrings(append(base.MimeTypes, parsed.MimeTypes...))
	if parsed.DateFrom != nil {
		base.DateFrom = parsed.DateFrom
	}
	if parsed.DateTo != nil {
		base.DateTo = parsed.DateTo
	}
	return base
}

func intentLocation(name string) *time.Location {
	name = strings.TrimSpace(name)
	if name == "" {
		name = defaultIntentTimezone
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		log.Printf("level=warn component=search event=intent_timezone_invalid timezone=%q err=%q", name, err)
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func endOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), t.Location())
}

func startOfWeek(t time.Time) time.Time {
	dayOffset := int(t.Weekday())
	if dayOffset == 0 {
		dayOffset = 7
	}
	return startOfDay(t).AddDate(0, 0, -(dayOffset - 1))
}

func monthRange(yearValue, monthValue string, loc *time.Location) (time.Time, time.Time, bool) {
	year, err := strconv.Atoi(yearValue)
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	month, err := strconv.Atoi(monthValue)
	if err != nil || month < 1 || month > 12 {
		return time.Time{}, time.Time{}, false
	}
	from := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, loc)
	to := endOfDay(from.AddDate(0, 1, -1))
	return from, to, true
}

func utcPtr(t time.Time) *time.Time {
	value := t.UTC()
	return &value
}

func parseOptionalTime(value *string) *time.Time {
	if value == nil || strings.TrimSpace(*value) == "" || strings.EqualFold(strings.TrimSpace(*value), "null") {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*value))
	if err != nil {
		return nil
	}
	return utcPtr(parsed)
}

func parseSmallNumber(value string) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if parsed, err := strconv.Atoi(value); err == nil {
		return parsed
	}
	switch value {
	case "一":
		return 1
	case "二", "两":
		return 2
	case "三":
		return 3
	case "四":
		return 4
	case "五":
		return 5
	case "六", "半":
		return 6
	case "七":
		return 7
	case "八":
		return 8
	case "九":
		return 9
	case "十":
		return 10
	default:
		return 0
	}
}

func containsExtensionIntent(query, ext string) bool {
	for _, index := range allSubstringIndexes(query, ext) {
		prev := runeBefore(query, index)
		next := runeAfter(query, index+len(ext))
		if prev == '.' {
			continue
		}
		if isASCIIAlphaNumeric(prev) || isASCIIAlphaNumeric(next) {
			continue
		}
		return true
	}
	return false
}

func removeExtensionIntent(query, ext string) string {
	lower := strings.ToLower(query)
	var builder strings.Builder
	last := 0
	for _, index := range allSubstringIndexes(lower, ext) {
		prev := runeBefore(lower, index)
		next := runeAfter(lower, index+len(ext))
		if prev == '.' || isASCIIAlphaNumeric(prev) || isASCIIAlphaNumeric(next) {
			continue
		}
		builder.WriteString(query[last:index])
		builder.WriteByte(' ')
		last = index + len(ext)
	}
	if last == 0 {
		return query
	}
	builder.WriteString(query[last:])
	return builder.String()
}

func allSubstringIndexes(text, substr string) []int {
	if substr == "" {
		return nil
	}
	var indexes []int
	offset := 0
	for {
		index := strings.Index(text[offset:], substr)
		if index < 0 {
			return indexes
		}
		absolute := offset + index
		indexes = append(indexes, absolute)
		offset = absolute + len(substr)
	}
}

func runeBefore(text string, byteIndex int) rune {
	if byteIndex <= 0 || byteIndex > len(text) {
		return 0
	}
	var last rune
	for _, r := range text[:byteIndex] {
		last = r
	}
	return last
}

func runeAfter(text string, byteIndex int) rune {
	if byteIndex < 0 || byteIndex >= len(text) {
		return 0
	}
	for _, r := range text[byteIndex:] {
		return r
	}
	return 0
}

func isASCIIAlphaNumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}

func removeWordToken(query, token string) string {
	return strings.Join(filterFields(query, func(field string) bool {
		return !strings.EqualFold(strings.Trim(field, " ,.;:!?()[]{}"), token)
	}), " ")
}

func filterFields(query string, keep func(string) bool) []string {
	fields := strings.Fields(query)
	filtered := make([]string, 0, len(fields))
	for _, field := range fields {
		if keep(field) {
			filtered = append(filtered, field)
		}
	}
	return filtered
}

func cleanupIntentText(text string) string {
	replacer := strings.NewReplacer(
		"帮我", " ",
		"帮忙", " ",
		"找一下", " ",
		"找一找", " ",
		"找找", " ",
		"查找", " ",
		"搜索", " ",
		"上传的", " ",
		"上传", " ",
		"文件", " ",
		"资料", " ",
		"一下", " ",
	)
	text = replacer.Replace(text)
	text = strings.Join(strings.Fields(text), " ")
	text = strings.Trim(text, " \t\r\n,，.。;；:：/\\-_的")
	return text
}

func normalizeExtension(ext string) string {
	ext = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(ext), "."))
	for _, r := range ext {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return ""
		}
	}
	return ext
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func looksLikeStructuredIntent(query string) bool {
	lower := strings.ToLower(query)
	keywords := []string{
		"今天", "昨天", "前天", "最近", "上周", "本周", "去年", "上个月", "这个月", "上传",
		"before", "after", "last", "recent", "uploaded", "file", "files",
	}
	for _, keyword := range keywords {
		if strings.Contains(lower, keyword) {
			return true
		}
	}
	return false
}

func timePtrString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format(time.RFC3339)
}
