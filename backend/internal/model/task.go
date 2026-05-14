package model

import "time"

// Task represents an asynchronous pipeline job that processes a file through
// the indexing pipeline: parse -> chunk -> embed -> index.
// Tasks are executed by a worker pool and report progress as a percentage.
type Task struct {
	ID         string    `json:"id"`
	FileID     string    `json:"file_id"`
	Type       string    `json:"type"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	Error      *string   `json:"error,omitempty"`
	RetryCount int       `json:"retry_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
