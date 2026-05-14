package model

import "time"

// SourceChunk is a retrieved text chunk with relevance metadata.
// It is returned by search queries and used as context in RAG conversations.
// Distance and Score indicate how well the chunk matches the query
// (lower distance = more similar; higher score = more relevant).
type SourceChunk struct {
	ID         string  `json:"id"`
	FileID     string  `json:"file_id"`
	FileName   string  `json:"file_name"`
	Heading    string  `json:"heading,omitempty"`
	ChunkIndex int     `json:"chunk_index"`
	ParentID   string  `json:"parent_chunk_id,omitempty"`
	Text       string  `json:"text"`
	Snippet    string  `json:"snippet"`
	Distance   float32 `json:"distance"`
	Score      float32 `json:"score"`
}

// Conversation represents an AI chat session. Current API modes are:
//   - "rag": retrieval-augmented question answering, optionally scoped to files
//   - "search": structured Smart Search
//
// Older database rows may contain "file_qa"; the store maps that legacy value
// back to "rag" when reading conversations.
type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Mode      string    `json:"mode"`
	FileIDs   []string  `json:"file_ids,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Message is a single turn in a conversation.
// Role must be "user" or "assistant". Sources contains the retrieved chunks
// that informed the assistant's response (only populated for assistant messages).
type Message struct {
	ID             string        `json:"id"`
	ConversationID string        `json:"conversation_id"`
	Role           string        `json:"role"`
	Content        string        `json:"content"`
	Sources        []SourceChunk `json:"sources,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}
