package indexing

import (
	"strings"

	"github.com/memodrive/backend/internal/parser"
)

type DocumentRef struct {
	ID   string
	Name string
}

type DocumentIndexOptions struct {
	ParentChunkSize int
	ChildChunkSize  int
	ChunkOverlap    int
}

type ChunkRecord struct {
	ID            string
	FileID        string
	FileName      string
	Heading       string
	ChunkIndex    int
	Text          string
	ParentChunkID string
	IsParent      bool
}

type DocumentIndexPlan struct {
	Hierarchy       *parser.HierarchicalChunks
	VectorIDs       []string
	VectorTexts     []string
	VectorMetadatas []map[string]any
	ChunkRecords    []ChunkRecord
}

func (p DocumentIndexPlan) ChildCount() int {
	if p.Hierarchy == nil {
		return 0
	}
	return len(p.Hierarchy.Children)
}

func BuildDocumentIndexPlan(file DocumentRef, doc *parser.ParsedDocument, opts DocumentIndexOptions) DocumentIndexPlan {
	hierarchy := parser.SplitDocumentHierarchical(doc, opts.ParentChunkSize, opts.ChildChunkSize, opts.ChunkOverlap)
	if hierarchy == nil {
		hierarchy = &parser.HierarchicalChunks{}
	}

	vectorIDs := make([]string, len(hierarchy.Children))
	vectorTexts := make([]string, len(hierarchy.Children))
	vectorMetadatas := make([]map[string]any, len(hierarchy.Children))
	source := documentSource(doc)
	for i, child := range hierarchy.Children {
		vectorIDs[i] = ChunkID(file.ID, child.Index)
		vectorTexts[i] = TextWithHeading(child.Heading, child.Text)
		vectorMetadatas[i] = (ChunkMetadata{
			FileID:        file.ID,
			FileName:      file.Name,
			Heading:       child.Heading,
			ChunkIndex:    child.Index,
			Source:        source,
			ParentChunkID: ParentIDForChild(file.ID, child),
		}).Map()
	}

	return DocumentIndexPlan{
		Hierarchy:       hierarchy,
		VectorIDs:       vectorIDs,
		VectorTexts:     vectorTexts,
		VectorMetadatas: vectorMetadatas,
		ChunkRecords:    chunkRecords(file, hierarchy, vectorTexts),
	}
}

func ParentIDForChild(fileID string, child parser.ChildChunk) string {
	if child.ParentIndex < 0 {
		return ""
	}
	return ParentChunkID(fileID, child.ParentIndex)
}

func TextWithHeading(heading, text string) string {
	heading = strings.TrimSpace(heading)
	text = strings.TrimSpace(text)
	if heading == "" {
		return text
	}
	if text == "" {
		return "## " + heading
	}
	return "## " + heading + "\n" + text
}

func documentSource(doc *parser.ParsedDocument) string {
	if doc != nil && doc.Meta != nil {
		if source := strings.TrimSpace(doc.Meta["source"]); source != "" {
			return source
		}
	}
	return "document"
}

func chunkRecords(file DocumentRef, hierarchy *parser.HierarchicalChunks, childTexts []string) []ChunkRecord {
	if hierarchy == nil {
		return nil
	}
	records := make([]ChunkRecord, 0, len(hierarchy.Parents)+len(hierarchy.Children))
	for _, parent := range hierarchy.Parents {
		records = append(records, ChunkRecord{
			ID:         ParentChunkID(file.ID, parent.Index),
			FileID:     file.ID,
			FileName:   file.Name,
			Heading:    parent.Heading,
			ChunkIndex: parent.Index,
			Text:       TextWithHeading(parent.Heading, parent.Text),
			IsParent:   true,
		})
	}
	for i, child := range hierarchy.Children {
		text := TextWithHeading(child.Heading, child.Text)
		if i < len(childTexts) && strings.TrimSpace(childTexts[i]) != "" {
			text = childTexts[i]
		}
		records = append(records, ChunkRecord{
			ID:            ChunkID(file.ID, child.Index),
			FileID:        file.ID,
			FileName:      file.Name,
			Heading:       child.Heading,
			ChunkIndex:    child.Index,
			Text:          text,
			ParentChunkID: ParentIDForChild(file.ID, child),
			IsParent:      false,
		})
	}
	return records
}
