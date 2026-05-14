package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
)

// AIHandler handles AI-powered endpoints: RAG chat and file search.
type AIHandler struct {
	llm       llm.Provider
	ragSvc    *service.RAGService
	searchSvc *service.SearchService
	convs     *service.ConversationService
}

// NewAIHandler creates a new AIHandler.
func NewAIHandler(llmProvider llm.Provider, rag *service.RAGService, search *service.SearchService, convs *service.ConversationService) *AIHandler {
	return &AIHandler{
		llm:       llmProvider,
		ragSvc:    rag,
		searchSvc: search,
		convs:     convs,
	}
}

func (h *AIHandler) Register(router fiber.Router) {
	router.Post("/ai/chat", h.chat)
	router.Post("/ai/search", h.search)
}

// chat handles the RAG chat endpoint. It:
//  1. Extracts the last user message as the search question
//  2. Ensures a conversation record exists (reuses existing or creates new)
//  3. Persists the user message
//  4. Calls RAGService.Chat to retrieve evidence and stream an LLM response
//  5. Streams SSE events: conversation ID, sources, delta chunks, done
//  6. After the stream ends, persists the assistant message with sources
func (h *AIHandler) chat(c *fiber.Ctx) error {
	started := time.Now()
	var request service.RAGRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid chat request")
	}
	question := chatQuestion(request)
	if question == "" {
		return aiError(service.ErrEmptyQuery)
	}
	if h.ragSvc == nil {
		log.Printf("level=warn component=ai event=chat_rag_unavailable fallback=direct_llm")
		return h.chatDirect(c, request, started)
	}

	convID := h.ensureConversation(c.UserContext(), request.ConversationID, "rag", question, request.FileIDs)
	if convID != "" {
		h.appendMessage(c.UserContext(), &model.Message{
			ConversationID: convID,
			Role:           "user",
			Content:        question,
		})
	}

	log.Printf("level=info component=ai event=rag_chat_begin conversation_id=%q messages=%d prompt_chars=%d file_filter=%d top_k=%d", convID, len(request.Messages), len([]rune(question)), len(request.FileIDs), request.TopK)
	stream, sources, err := h.ragSvc.Chat(c.UserContext(), request)
	if err != nil {
		log.Printf("level=error component=ai event=rag_chat_failed conversation_id=%q duration_ms=%d err=%q", convID, time.Since(started).Milliseconds(), err)
		return aiError(err)
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		chunks := 0
		bytesWritten := 0
		var answer strings.Builder
		if convID != "" {
			writeSSEEvent(w, "conversation", fiber.Map{"id": convID})
		}
		writeSSEEvent(w, "sources", fiber.Map{"sources": sources})
		for chunk := range stream {
			chunks++
			bytesWritten += len(chunk)
			answer.WriteString(chunk)
			writeSSEData(w, fiber.Map{"delta": chunk})
		}
		_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		_ = w.Flush()
		if convID != "" {
			h.appendMessage(context.Background(), &model.Message{
				ConversationID: convID,
				Role:           "assistant",
				Content:        answer.String(),
				Sources:        sources,
			})
		}
		log.Printf("level=info component=rag event=chat_stream_complete conversation_id=%q sources=%d chunks=%d bytes=%d duration_ms=%d", convID, len(sources), chunks, bytesWritten, time.Since(started).Milliseconds())
	})
	return nil
}

func (h *AIHandler) chatDirect(c *fiber.Ctx, request service.RAGRequest, started time.Time) error {
	if h.llm == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "llm provider is not configured")
	}
	messages := request.Messages
	if len(messages) == 0 && request.Prompt != "" {
		messages = []llm.Message{{Role: "user", Content: request.Prompt}}
	}
	if len(messages) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "prompt or messages is required")
	}
	log.Printf("level=info component=ai event=chat_begin provider=%s messages=%d", h.llm.Name(), len(messages))
	stream, err := h.llm.Chat(c.UserContext(), messages)
	if err != nil {
		log.Printf("level=error component=ai event=chat_provider_failed provider=%s messages=%d duration_ms=%d err=%q", h.llm.Name(), len(messages), time.Since(started).Milliseconds(), err)
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}

	c.Set("Content-Type", "text/event-stream")
	c.Set("Cache-Control", "no-cache")
	c.Set("Connection", "keep-alive")
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		chunks := 0
		bytesWritten := 0
		for chunk := range stream {
			chunks++
			bytesWritten += len(chunk)
			writeSSEData(w, fiber.Map{"delta": chunk})
		}
		_, _ = fmt.Fprint(w, "event: done\ndata: {}\n\n")
		_ = w.Flush()
		log.Printf("level=info component=ai event=chat_stream_complete provider=%s chunks=%d bytes=%d duration_ms=%d", h.llm.Name(), chunks, bytesWritten, time.Since(started).Milliseconds())
	})
	return nil
}

// search handles the semantic search endpoint. Flow:
//  1. Parse request, extract query
//  2. Ensure conversation record, persist user message
//  3. Call SearchService.Search for hybrid (vector + BM25 + RRF fusion) retrieval
//  4. Persist a synthetic assistant message summarizing result count
//  5. Return JSON response with sources
func (h *AIHandler) search(c *fiber.Ctx) error {
	started := time.Now()
	if h.searchSvc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search service is not configured")
	}
	var request service.SearchRequest
	if err := c.BodyParser(&request); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid search request")
	}
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return aiError(service.ErrEmptyQuery)
	}
	convID := h.ensureConversation(c.UserContext(), request.ConversationID, "search", query, request.FileIDs)
	if convID != "" {
		h.appendMessage(c.UserContext(), &model.Message{
			ConversationID: convID,
			Role:           "user",
			Content:        query,
		})
	}
	response, err := h.searchSvc.Search(c.UserContext(), request)
	if err != nil {
		log.Printf("level=error component=ai event=search_failed conversation_id=%q duration_ms=%d err=%q", convID, time.Since(started).Milliseconds(), err)
		return aiError(err)
	}
	response.ConversationID = convID
	if convID != "" {
		h.appendMessage(context.Background(), &model.Message{
			ConversationID: convID,
			Role:           "assistant",
			Content:        fmt.Sprintf("找到 %d 条相关结果", len(response.Results)),
			Sources:        response.Results,
		})
	}
	log.Printf("level=info component=ai event=search_complete conversation_id=%q results=%d duration_ms=%d", convID, len(response.Results), time.Since(started).Milliseconds())
	return c.JSON(response)
}

func (h *AIHandler) ensureConversation(ctx context.Context, id, mode, question string, fileIDs []string) string {
	if h.convs == nil {
		return ""
	}
	convID, err := h.convs.EnsureConversation(ctx, id, mode, question, fileIDs)
	if err != nil {
		log.Printf("level=warn component=ai event=conversation_ensure_failed mode=%s err=%q", mode, err)
		return ""
	}
	return convID
}

func (h *AIHandler) appendMessage(ctx context.Context, msg *model.Message) {
	if h.convs == nil || msg == nil || msg.ConversationID == "" {
		return
	}
	if err := h.convs.Append(ctx, msg); err != nil {
		log.Printf("level=warn component=ai event=conversation_append_failed conversation_id=%q role=%s err=%q", msg.ConversationID, msg.Role, err)
	}
}

// chatQuestion extracts the user's question from a RAGRequest.
// It uses Prompt if set, otherwise scans Messages backwards for the last "user" role.
func chatQuestion(request service.RAGRequest) string {
	if prompt := strings.TrimSpace(request.Prompt); prompt != "" {
		return prompt
	}
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(strings.TrimSpace(request.Messages[i].Role), "user") {
			return strings.TrimSpace(request.Messages[i].Content)
		}
	}
	return ""
}

// aiError maps service-layer errors to HTTP status codes.
// ErrEmptyQuery → 400, ErrServiceUnavailable → 503, others → 502.
func aiError(err error) error {
	switch {
	case errors.Is(err, service.ErrEmptyQuery):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrServiceUnavailable):
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	default:
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
}

func writeSSEData(w *bufio.Writer, value any) {
	payload, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", payload)
	_ = w.Flush()
}

func writeSSEEvent(w *bufio.Writer, event string, value any) {
	payload, _ := json.Marshal(value)
	_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, payload)
	_ = w.Flush()
}
