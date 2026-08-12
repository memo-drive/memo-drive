package model

import "time"

const (
	FileMutationKindUploadCreate   = "upload_create"
	FileMutationKindUploadReplace  = "upload_replace"
	FileMutationKindCopyCreate     = "copy_create"
	FileMutationKindCopyReplace    = "copy_replace"
	FileMutationKindVersionRestore = "version_restore"

	FileMutationStatePrepared    = "prepared"
	FileMutationStateFSApplied   = "fs_applied"
	FileMutationStateDBCommitted = "db_committed"
	FileMutationStateFinalized   = "finalized"
	FileMutationStateFailed      = "failed"
)

// FileMutation records the recoverable boundary between storage and SQLite.
type FileMutation struct {
	ID               string
	Kind             string
	State            string
	VirtualPath      string
	TargetFileID     string
	StagedPath       string
	OldStoragePath   string
	FinalStoragePath string
	Error            string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}
