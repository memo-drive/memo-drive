package model

import "time"

const (
	FileVersionSourceUploadReplace  = "upload_replace"
	FileVersionSourceMarkdownSave   = "markdown_save"
	FileVersionSourceWebDAVPut      = "webdav_put"
	FileVersionSourceCopyReplace    = "copy_replace"
	FileVersionSourceVersionRestore = "version_restore"
)

// FileVersion is a historical content snapshot of one File.
type FileVersion struct {
	ID             string    `json:"id"`
	FileID         string    `json:"file_id"`
	VersionNo      int       `json:"version_no"`
	StoragePath    string    `json:"-"`
	Size           int64     `json:"size"`
	MimeType       string    `json:"mime_type,omitempty"`
	SHA256         string    `json:"sha256,omitempty"`
	ChecksumStatus string    `json:"checksum_status"`
	Source         string    `json:"source"`
	CreatedAt      time.Time `json:"created_at"`
}
