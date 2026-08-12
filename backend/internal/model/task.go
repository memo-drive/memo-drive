package model

import "time"

// Task represents an asynchronous pipeline job that processes a file through
// the indexing pipeline: parse -> chunk -> embed -> index.
// Tasks are executed by a worker pool and report progress as a percentage.
type Task struct {
	ID            string    `json:"id"`
	FileID        string    `json:"file_id"`
	Type          string    `json:"type"`
	Status        string    `json:"status"`
	Progress      int       `json:"progress"`
	Error         *string   `json:"error,omitempty"`
	RetryCount    int       `json:"retry_count"`
	RetryOfTaskID string    `json:"retry_of_task_id,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TaskFileSummary identifies the File whose content a pipeline Task processes.
type TaskFileSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	MimeType string `json:"mime_type"`
	Status   string `json:"status"`
}

// TaskListItem combines the Task audit record with its current File summary.
type TaskListItem struct {
	Task
	File TaskFileSummary `json:"file"`
}

// TaskListPage is the stable response envelope for Task collection queries.
type TaskListPage struct {
	Items      []TaskListItem `json:"items"`
	NextCursor string         `json:"next_cursor"`
	HasMore    bool           `json:"has_more"`
}
