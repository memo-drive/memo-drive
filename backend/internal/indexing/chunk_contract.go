// Package indexing orchestrates the transformation of parsed documents into
// vector-indexable chunks. It defines the chunk ID contract, builds hierarchical
// indexing plans from parsed documents, and runs the vector indexing pipeline
// (embedding -> upsert).
package indexing

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	MetadataFileID        = "file_id"
	MetadataFileName      = "file_name"
	MetadataHeading       = "heading"
	MetadataChunkIndex    = "chunk_index"
	MetadataSource        = "source"
	MetadataParentChunkID = "parent_chunk_id"
)

// ChunkMetadata is the metadata attached to each vector in the vector store.
// It enables reconstruction of chunk identity and parent-child relationships
// from query results.
type ChunkMetadata struct {
	FileID        string
	FileName      string
	Heading       string
	ChunkIndex    int
	Source        string
	ParentChunkID string
}

// ChunkID returns the vector store ID for a child chunk in the format "fileID#index".
func ChunkID(fileID string, index int) string {
	return fmt.Sprintf("%s#%d", fileID, index)
}

// ParentChunkID returns the vector store ID for a parent chunk in the format "fileID#parent-index".
func ParentChunkID(fileID string, index int) string {
	return fmt.Sprintf("%s#parent-%d", fileID, index)
}

// ChunkIDs generates a slice of sequential child chunk IDs for a file.
func ChunkIDs(fileID string, count int) []string {
	if count <= 0 {
		return nil
	}
	ids := make([]string, count)
	for i := 0; i < count; i++ {
		ids[i] = ChunkID(fileID, i)
	}
	return ids
}

// Map serializes the metadata into a map suitable for the vector store API.
func (m ChunkMetadata) Map() map[string]any {
	return map[string]any{
		MetadataFileID:        m.FileID,
		MetadataFileName:      m.FileName,
		MetadataHeading:       m.Heading,
		MetadataChunkIndex:    m.ChunkIndex,
		MetadataSource:        m.Source,
		MetadataParentChunkID: m.ParentChunkID,
	}
}

// ChunkMetadataFromMap deserializes metadata from a vector store query result.
func ChunkMetadataFromMap(metadata map[string]any) ChunkMetadata {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return ChunkMetadata{
		FileID:        metadataString(metadata, MetadataFileID),
		FileName:      metadataString(metadata, MetadataFileName),
		Heading:       metadataString(metadata, MetadataHeading),
		ChunkIndex:    metadataInt(metadata, MetadataChunkIndex, -1),
		Source:        metadataString(metadata, MetadataSource),
		ParentChunkID: metadataString(metadata, MetadataParentChunkID),
	}
}

func metadataString(metadata map[string]any, key string) string {
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	case json.Number:
		return typed.String()
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func metadataInt(metadata map[string]any, key string, fallback int) int {
	value, ok := metadata[key]
	if !ok || value == nil {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return typed
	case int8:
		return int(typed)
	case int16:
		return int(typed)
	case int32:
		return int(typed)
	case int64:
		return int(typed)
	case uint:
		if uint64(typed) > uint64(maxIntValue()) {
			return fallback
		}
		return int(typed)
	case uint8:
		return int(typed)
	case uint16:
		return int(typed)
	case uint32:
		return int(typed)
	case uint64:
		if typed > uint64(maxIntValue()) {
			return fallback
		}
		return int(typed)
	case float32:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return int(parsed)
		}
		if parsed, err := typed.Float64(); err == nil {
			return int(parsed)
		}
	case string:
		var parsed json.Number = json.Number(strings.TrimSpace(typed))
		if value, err := parsed.Int64(); err == nil {
			return int(value)
		}
	}
	return fallback
}

func maxIntValue() int {
	return int(^uint(0) >> 1)
}
