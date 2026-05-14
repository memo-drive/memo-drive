package service

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
)

const maxConversationMessages = 10

const condenseSystemPrompt = `给定一段对话历史和一个后续问题，将后续问题改写为一个独立的检索查询。
只输出改写后的查询文本，不要解释。如果问题已经独立，原样返回。`

// RAGService implements retrieval-augmented generation: it searches for relevant
// chunks, builds a context prompt, and streams an LLM response.
type RAGService struct {
	cfg    *config.Config
	llm    llm.Provider
	search *SearchService
}

// NewRAGService creates a new RAGService.
func NewRAGService(cfg *config.Config, llmProvider llm.Provider, search *SearchService) *RAGService {
	return &RAGService{
		cfg:    cfg,
		llm:    llmProvider,
		search: search,
	}
}

func (s *RAGService) Chat(ctx context.Context, req RAGRequest) (<-chan string, []SourceChunk, error) {
	started := time.Now()
	question := extractQuestion(req)
	if strings.TrimSpace(question) == "" {
		return nil, nil, ErrEmptyQuery
	}
	if s == nil || s.llm == nil {
		return nil, nil, fmt.Errorf("%w: llm provider is not configured", ErrServiceUnavailable)
	}
	if s.search == nil {
		return nil, nil, fmt.Errorf("%w: search service is not configured", ErrServiceUnavailable)
	}

	topK := s.ragTopK(req.TopK)
	originalQuestion := question
	question = s.condenseQuestion(ctx, req.Messages, question)
	log.Printf("level=info component=rag event=chat_begin question_chars=%d messages=%d top_k=%d file_filter=%d provider=%s",
		len([]rune(question)), len(req.Messages), topK, len(req.FileIDs), s.llm.Name())

	searchStarted := time.Now()
	searchResult, err := s.search.Search(ctx, SearchRequest{
		Query:   question,
		FileIDs: req.FileIDs,
		TopK:    topK,
	})
	if err != nil {
		log.Printf("level=error component=rag event=chat_failed stage=search question_chars=%d duration_ms=%d err=%q", len([]rune(question)), time.Since(started).Milliseconds(), err)
		return nil, nil, err
	}
	sources := searchResult.Results
	messages, contextChars, usedSources := buildRAGMessages(req, originalQuestion, sources, s.maxContextChars())
	log.Printf("level=info component=rag event=retrieval_complete sources=%d used_sources=%d context_chars=%d duration_ms=%d",
		len(sources), usedSources, contextChars, time.Since(searchStarted).Milliseconds())

	stream, err := s.llm.Chat(ctx, messages)
	if err != nil {
		log.Printf("level=error component=rag event=chat_failed stage=llm question_chars=%d sources=%d duration_ms=%d err=%q", len([]rune(question)), len(sources), time.Since(started).Milliseconds(), err)
		return nil, nil, err
	}
	return stream, sources, nil
}

func (s *RAGService) condenseQuestion(ctx context.Context, messages []llm.Message, question string) string {
	question = strings.TrimSpace(question)
	if question == "" || s == nil || s.llm == nil || s.cfg == nil || !s.cfg.RAG.QueryCondense {
		return question
	}
	recent := recentNonSystemMessages(messages, 4)
	if len(recent) <= 1 {
		return question
	}

	var builder strings.Builder
	builder.WriteString("对话历史:\n")
	for _, message := range recent[:len(recent)-1] {
		builder.WriteString(message.Role)
		builder.WriteString(": ")
		builder.WriteString(message.Content)
		builder.WriteString("\n")
	}
	builder.WriteString("\n后续问题: ")
	builder.WriteString(question)
	builder.WriteString("\n改写查询:")

	condensed, err := s.llm.Complete(ctx, []llm.Message{
		{Role: "system", Content: condenseSystemPrompt},
		{Role: "user", Content: builder.String()},
	})
	if err != nil {
		log.Printf("level=warn component=rag event=condense_failed question_chars=%d err=%q", len([]rune(question)), err)
		return question
	}
	result := strings.TrimSpace(condensed)
	if result == "" {
		return question
	}
	log.Printf("level=info component=rag event=condense original_chars=%d condensed_chars=%d changed=%t",
		len([]rune(question)), len([]rune(result)), result != question)
	return result
}

func extractQuestion(req RAGRequest) string {
	if prompt := strings.TrimSpace(req.Prompt); prompt != "" {
		return prompt
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(req.Messages[i].Role), "user") {
			return strings.TrimSpace(req.Messages[i].Content)
		}
	}
	return ""
}

func buildRAGMessages(req RAGRequest, question string, sources []SourceChunk, maxContextChars int) ([]llm.Message, int, int) {
	contextText, contextChars, usedSources := buildContext(sources, maxContextChars)
	systemPrompt := `你是 MemoDrive 的私人云盘 AI 助手。请优先基于“资料片段”回答用户问题。
如果资料片段不足以回答，请明确说明“不确定”或“当前资料中没有找到”。
不要编造文件内容、页码或不存在的来源。
回答中如引用资料，请使用 [1]、[2] 这样的编号标注来源。`
	if strings.TrimSpace(contextText) == "" {
		contextText = "资料片段：\n当前资料中没有检索到相关片段。"
	} else {
		contextText = "资料片段：\n" + contextText
	}
	messages := []llm.Message{{
		Role:    "system",
		Content: systemPrompt + "\n\n" + contextText,
	}}
	conversation := recentNonSystemMessages(req.Messages, maxConversationMessages)
	if len(conversation) == 0 {
		conversation = append(conversation, llm.Message{Role: "user", Content: question})
	} else if prompt := strings.TrimSpace(req.Prompt); prompt != "" && !lastUserMessageEquals(conversation, prompt) {
		conversation = append(conversation, llm.Message{Role: "user", Content: prompt})
	}
	messages = append(messages, conversation...)
	return messages, contextChars, usedSources
}

func buildContext(sources []SourceChunk, maxContextChars int) (string, int, int) {
	if maxContextChars <= 0 {
		return "", 0, 0
	}
	var builder strings.Builder
	used := 0
	for i, source := range sources {
		block := formatSourceBlock(i+1, source)
		nextLen := len([]rune(builder.String())) + len([]rune(block))
		if nextLen > maxContextChars {
			break
		}
		builder.WriteString(block)
		used++
	}
	return strings.TrimSpace(builder.String()), len([]rune(builder.String())), used
}

func formatSourceBlock(index int, source SourceChunk) string {
	heading := source.Heading
	if heading == "" {
		heading = "未命名段落"
	}
	fileName := source.FileName
	if fileName == "" {
		fileName = source.FileID
	}
	text := strings.TrimSpace(source.Text)
	if text == "" {
		text = strings.TrimSpace(source.Snippet)
	}
	return fmt.Sprintf("[%d] 文件: %s / 标题: %s / Chunk: %d\n内容: %s\n\n", index, fileName, heading, source.ChunkIndex, text)
}

func recentNonSystemMessages(messages []llm.Message, limit int) []llm.Message {
	if limit <= 0 || len(messages) == 0 {
		return nil
	}
	filtered := make([]llm.Message, 0, len(messages))
	for _, message := range messages {
		role := strings.TrimSpace(strings.ToLower(message.Role))
		content := strings.TrimSpace(message.Content)
		if role == "" || content == "" || role == "system" {
			continue
		}
		filtered = append(filtered, llm.Message{Role: role, Content: content})
	}
	if len(filtered) <= limit {
		return filtered
	}
	return filtered[len(filtered)-limit:]
}

func lastUserMessageEquals(messages []llm.Message, prompt string) bool {
	prompt = strings.TrimSpace(prompt)
	for i := len(messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(messages[i].Role), "user") {
			return strings.TrimSpace(messages[i].Content) == prompt
		}
	}
	return false
}

func (s *RAGService) ragTopK(requested int) int {
	if requested <= 0 {
		if s != nil && s.cfg != nil && s.cfg.RAG.TopK > 0 {
			requested = s.cfg.RAG.TopK
		} else {
			requested = defaultRAGTopK
		}
	}
	if requested > maxRAGTopK {
		return maxRAGTopK
	}
	return requested
}

func (s *RAGService) maxContextChars() int {
	if s == nil || s.cfg == nil || s.cfg.RAG.MaxContextChars <= 0 {
		return 6000
	}
	return s.cfg.RAG.MaxContextChars
}
