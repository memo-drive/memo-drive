package service

import (
	"time"

	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
)

type SourceChunk = model.SourceChunk

type SearchRequest struct {
	Query          string        `json:"query"`
	FileIDs        []string      `json:"file_ids,omitempty"`
	TopK           int           `json:"top_k,omitempty"`
	ConversationID string        `json:"conversation_id,omitempty"`
	Intent         *SearchIntent `json:"intent,omitempty"`
}

type SearchResponse struct {
	ConversationID string        `json:"conversation_id,omitempty"`
	Query          string        `json:"query"`
	Results        []SourceChunk `json:"results"`
	Intent         *SearchIntent `json:"intent,omitempty"`
}

type FileSearchRequest struct {
	Query      string `json:"query"`
	Path       string `json:"path,omitempty"`
	MimePrefix string `json:"mime,omitempty"`
	Semantic   bool   `json:"semantic,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

type FileSearchHit struct {
	File       *model.File `json:"file"`
	MatchTypes []string    `json:"match_types"`
	Snippet    string      `json:"snippet,omitempty"`
	Score      float32     `json:"score"`
}

type FileSearchResponse struct {
	Query    string          `json:"query"`
	Total    int             `json:"total"`
	Hits     []FileSearchHit `json:"hits"`
	Semantic bool            `json:"semantic"`
	Intent   *SearchIntent   `json:"intent,omitempty"`
}

type SearchIntent struct {
	TextQuery  string     `json:"text_query"`
	MimeTypes  []string   `json:"mime_types,omitempty"`
	Extensions []string   `json:"extensions,omitempty"`
	DateFrom   *time.Time `json:"date_from,omitempty"`
	DateTo     *time.Time `json:"date_to,omitempty"`
	Original   string     `json:"original"`
}

func (i SearchIntent) HasFilters() bool {
	return len(i.MimeTypes) > 0 || len(i.Extensions) > 0 || i.DateFrom != nil || i.DateTo != nil
}

func (i SearchIntent) PrimaryMime() string {
	if len(i.MimeTypes) == 0 {
		return ""
	}
	return i.MimeTypes[0]
}

type RAGRequest struct {
	Prompt         string        `json:"prompt"`
	Messages       []llm.Message `json:"messages,omitempty"`
	FileIDs        []string      `json:"file_ids,omitempty"`
	TopK           int           `json:"top_k,omitempty"`
	ConversationID string        `json:"conversation_id,omitempty"`
}
