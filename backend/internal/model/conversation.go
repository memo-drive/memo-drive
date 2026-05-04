package model

import "time"

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

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Mode      string    `json:"mode"`
	FileIDs   []string  `json:"file_ids,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Message struct {
	ID             string        `json:"id"`
	ConversationID string        `json:"conversation_id"`
	Role           string        `json:"role"`
	Content        string        `json:"content"`
	Sources        []SourceChunk `json:"sources,omitempty"`
	CreatedAt      time.Time     `json:"created_at"`
}
