package model

const (
	FileStatusUploaded   = "uploaded"
	FileStatusProcessing = "processing"
	FileStatusReady      = "ready"
	FileStatusFailed     = "failed"

	TaskStatusPending    = "pending"
	TaskStatusProcessing = "processing"
	TaskStatusDone       = "done"
	TaskStatusFailed     = "failed"

	UploadStatusUploading = "uploading"
	UploadStatusMerging   = "merging"
	UploadStatusDone      = "done"
	UploadStatusCancelled = "cancelled"
	UploadStatusExpired   = "expired"
	UploadStatusFailed    = "failed"
)
