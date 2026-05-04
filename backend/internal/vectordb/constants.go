package vectordb

import "fmt"

const DefaultCollection = "memodrive"

const MetadataParentChunkID = "parent_chunk_id"

func ChunkID(fileID string, index int) string {
	return fmt.Sprintf("%s#%d", fileID, index)
}

func ParentChunkID(fileID string, index int) string {
	return fmt.Sprintf("%s#parent-%d", fileID, index)
}

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
