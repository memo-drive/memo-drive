package model

// File processing lifecycle states.
const (
	FileStatusUploaded   = "uploaded"   // file has been uploaded but not yet processed
	FileStatusProcessing = "processing" // pipeline is actively working on the file
	FileStatusReady      = "ready"      // pipeline finished; searchable when chunks were produced
	FileStatusFailed     = "failed"     // pipeline encountered an unrecoverable error

	// Task execution states for pipeline jobs.
	TaskStatusPending    = "pending"
	TaskStatusProcessing = "processing"
	TaskStatusDone       = "done"
	TaskStatusFailed     = "failed"

	// Upload session lifecycle states.
	UploadStatusUploading = "uploading"
	UploadStatusMerging   = "merging" // chunks are being assembled into the final file
	UploadStatusDone      = "done"
	UploadStatusCancelled = "cancelled"
	UploadStatusExpired   = "expired"
	UploadStatusFailed    = "failed"
)
