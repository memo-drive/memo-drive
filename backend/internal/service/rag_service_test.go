package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
)

func TestRAGChatBuildsContextAndCallsLLM(t *testing.T) {
	provider := &mockSearchProvider{}
	search := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, nil, provider, &mockVectorStore{queryResult: sampleQueryResult()})
	rag := NewRAGService(&config.Config{RAG: config.RAGConfig{TopK: 2, MaxContextChars: 4000}}, provider, search)

	stream, sources, err := rag.Chat(context.Background(), RAGRequest{Prompt: "How do I troubleshoot login?"})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(sources) != 2 {
		t.Fatalf("expected 2 sources, got %d", len(sources))
	}
	if !provider.chatCalled {
		t.Fatal("expected LLM chat to be called")
	}
	if len(provider.chatMsgs) < 2 {
		t.Fatalf("expected system and user messages, got %#v", provider.chatMsgs)
	}
	if provider.chatMsgs[0].Role != "system" || !strings.Contains(provider.chatMsgs[0].Content, "资料片段") || !strings.Contains(provider.chatMsgs[0].Content, "Guide.md") {
		t.Fatalf("unexpected system prompt: %#v", provider.chatMsgs[0])
	}
	if provider.chatMsgs[len(provider.chatMsgs)-1].Content != "How do I troubleshoot login?" {
		t.Fatalf("expected user prompt to be preserved, got %#v", provider.chatMsgs)
	}
	chunk := <-stream
	if chunk != "ok" {
		t.Fatalf("expected stream chunk ok, got %q", chunk)
	}
}

func TestRAGChatExtractsLastUserMessage(t *testing.T) {
	provider := &mockSearchProvider{}
	search := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, nil, provider, &mockVectorStore{queryResult: sampleQueryResult()})
	rag := NewRAGService(&config.Config{RAG: config.RAGConfig{TopK: 2, MaxContextChars: 4000}}, provider, search)

	_, _, err := rag.Chat(context.Background(), RAGRequest{Messages: []llm.Message{
		{Role: "user", Content: "old question"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "current question"},
	}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if got := provider.chatMsgs[len(provider.chatMsgs)-1].Content; got != "current question" {
		t.Fatalf("expected current question, got %q", got)
	}
}

func TestRAGChatCondensesMultiTurnQuestionBeforeSearch(t *testing.T) {
	provider := &mockSearchProvider{completeResult: "login troubleshooting steps"}
	search := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, nil, provider, &mockVectorStore{queryResult: sampleQueryResult()})
	rag := NewRAGService(&config.Config{RAG: config.RAGConfig{TopK: 2, MaxContextChars: 4000, QueryCondense: true}}, provider, search)

	_, _, err := rag.Chat(context.Background(), RAGRequest{Messages: []llm.Message{
		{Role: "user", Content: "How do I fix login?"},
		{Role: "assistant", Content: "Check the guide."},
		{Role: "user", Content: "What about that error?"},
	}})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if provider.completeCalls != 1 {
		t.Fatalf("expected condense Complete call, got %d", provider.completeCalls)
	}
	if len(provider.embedTexts) == 0 || provider.embedTexts[0] != "login troubleshooting steps" {
		t.Fatalf("expected condensed query to be embedded, got %#v", provider.embedTexts)
	}
	if got := provider.chatMsgs[len(provider.chatMsgs)-1].Content; got != "What about that error?" {
		t.Fatalf("expected original user question to be preserved in chat, got %q", got)
	}
}

func TestRAGChatLimitsContext(t *testing.T) {
	sources := []SourceChunk{
		{FileName: "A.md", Heading: "A", ChunkIndex: 0, Text: strings.Repeat("a", 80)},
		{FileName: "B.md", Heading: "B", ChunkIndex: 1, Text: strings.Repeat("b", 80)},
	}
	messages, contextChars, usedSources := buildRAGMessages(RAGRequest{Prompt: "question"}, "question", sources, 140)
	if usedSources != 1 {
		t.Fatalf("expected one source inside context limit, got %d", usedSources)
	}
	if contextChars <= 0 || contextChars > 140 {
		t.Fatalf("unexpected context chars: %d", contextChars)
	}
	if strings.Contains(messages[0].Content, "B.md") {
		t.Fatalf("second source should have been truncated: %s", messages[0].Content)
	}
}

func TestRAGChatCallsLLMWithoutSources(t *testing.T) {
	provider := &mockSearchProvider{}
	search := NewSearchService(&config.Config{RAG: config.RAGConfig{SearchTopK: 5}}, nil, provider, &mockVectorStore{queryResult: nil})
	rag := NewRAGService(&config.Config{RAG: config.RAGConfig{TopK: 2, MaxContextChars: 4000}}, provider, search)

	_, sources, err := rag.Chat(context.Background(), RAGRequest{Prompt: "unknown"})
	if err != nil {
		t.Fatalf("Chat returned error: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("expected no sources, got %#v", sources)
	}
	if !provider.chatCalled {
		t.Fatal("expected LLM chat to be called without sources")
	}
	if !strings.Contains(provider.chatMsgs[0].Content, "没有检索到相关片段") {
		t.Fatalf("expected no-source prompt, got %q", provider.chatMsgs[0].Content)
	}
}

func TestRAGChatDoesNotCallLLMWhenSearchFails(t *testing.T) {
	provider := &mockSearchProvider{}
	search := NewSearchService(&config.Config{}, nil, provider, &mockVectorStore{queryErr: errors.New("chroma down")})
	rag := NewRAGService(&config.Config{RAG: config.RAGConfig{TopK: 2, MaxContextChars: 4000}}, provider, search)

	_, _, err := rag.Chat(context.Background(), RAGRequest{Prompt: "question"})
	if err == nil {
		t.Fatal("expected search error")
	}
	if provider.chatCalled {
		t.Fatal("LLM chat should not be called when search fails")
	}
}
