package model

import "time"

const (
	FileCopyOperationStateRunning   = "running"
	FileCopyOperationStateCompleted = "completed"
	FileCopyOperationStateFailed    = "failed"
)

// FileCopyOperation tracks the recoverable lifetime of one recursive Folder Copy.
type FileCopyOperation struct {
	ID         string
	SourceID   string
	RootFileID string
	State      string
	Error      string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
