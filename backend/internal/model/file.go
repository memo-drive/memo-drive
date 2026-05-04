package model

import "time"

type File struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Path         string        `json:"path"`
	StoragePath  string        `json:"storage_path"`
	Size         int64         `json:"size"`
	MimeType     string        `json:"mime_type"`
	IsDir        bool          `json:"is_dir"`
	ParentID     *string       `json:"parent_id,omitempty"`
	Status       string        `json:"status"`
	ChunkCount   int           `json:"chunk_count"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	DeletedAt    *time.Time    `json:"deleted_at,omitempty"`
	OriginalPath *string       `json:"original_path,omitempty"`
	OriginalName *string       `json:"original_name,omitempty"`
	TrashRootID  *string       `json:"trash_root_id,omitempty"`
	Metadata     *FileMetadata `json:"metadata,omitempty"`
}

type FileMetadata struct {
	FileID        string    `json:"file_id"`
	MetaJSON      string    `json:"meta_json"`
	ThumbnailPath *string   `json:"thumbnail_path,omitempty"`
	ExtractedAt   time.Time `json:"extracted_at"`
}

type UploadSession struct {
	ID             string    `json:"id"`
	FileName       string    `json:"file_name"`
	FileSize       int64     `json:"file_size"`
	ChunkSize      int64     `json:"chunk_size"`
	UploadedChunks []int     `json:"uploaded_chunks"`
	DestPath       string    `json:"dest_path"`
	Status         string    `json:"status"`
	CreatedAt      time.Time `json:"created_at"`
	ExpiresAt      time.Time `json:"expires_at"`
}
