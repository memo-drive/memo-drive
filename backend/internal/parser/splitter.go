package parser

import (
	"log"
	"strings"
	"time"
)

const (
	defaultChunkSize    = 500
	defaultChunkOverlap = 100
	maxLookback         = 100
)

// SplitOptions controls how text is split into chunks.
type SplitOptions struct {
	ChunkSize int // Target characters per chunk.
	Overlap   int // Characters shared by adjacent chunks.
}

// Chunk is a contiguous piece of text with a heading and positional index.
type Chunk struct {
	Text    string
	Index   int
	Heading string
}

// HierarchicalChunks holds a two-level chunk hierarchy: large parent chunks
// (for context) and smaller child chunks (for precise retrieval).
type HierarchicalChunks struct {
	Parents  []Chunk
	Children []ChildChunk
}

// ChildChunk extends Chunk with a reference to its parent chunk index.
type ChildChunk struct {
	Chunk
	ParentIndex int
}

// SplitDocument divides a parsed document into chunks respecting section boundaries.
// Each chunk is at most opts.ChunkSize characters, with opts.Overlap characters
// shared between adjacent chunks.
func SplitDocument(doc *ParsedDocument, opts SplitOptions) []Chunk {
	started := time.Now()
	if doc == nil {
		log.Printf("level=debug component=parser event=split_skipped reason=nil_document")
		return nil
	}
	opts = normalizeSplitOptions(opts)

	var chunks []Chunk
	nextIndex := 0
	for _, section := range doc.Sections {
		body := strings.TrimSpace(section.Body)
		if body == "" {
			continue
		}
		if runeLen(body) <= opts.ChunkSize {
			chunks = append(chunks, Chunk{
				Text:    body,
				Index:   nextIndex,
				Heading: section.Heading,
			})
			nextIndex++
			continue
		}
		chunks = append(chunks, splitRunes(body, section.Heading, opts, &nextIndex)...)
	}

	if len(chunks) > 0 {
		log.Printf("level=info component=parser event=split_complete source=sections sections=%d chunks=%d chunk_size=%d overlap=%d duration_ms=%d",
			len(doc.Sections), len(chunks), opts.ChunkSize, opts.Overlap, time.Since(started).Milliseconds())
		return chunks
	}
	chunks = splitRunes(doc.Text, "", opts, &nextIndex)
	log.Printf("level=info component=parser event=split_complete source=text chars=%d chunks=%d chunk_size=%d overlap=%d duration_ms=%d",
		len([]rune(doc.Text)), len(chunks), opts.ChunkSize, opts.Overlap, time.Since(started).Milliseconds())
	return chunks
}

// SplitDocumentHierarchical creates a two-level chunk hierarchy: parents (larger)
// provide retrieval context, and children (smaller) enable precise matching.
func SplitDocumentHierarchical(doc *ParsedDocument, parentSize, childSize, overlap int) *HierarchicalChunks {
	parentOpts := SplitOptions{ChunkSize: parentSize, Overlap: overlap}
	parents := SplitDocument(doc, parentOpts)
	childOpts := SplitOptions{ChunkSize: childSize, Overlap: overlap / 2}

	children := make([]ChildChunk, 0, len(parents))
	nextChildIndex := 0
	for parentIndex, parent := range parents {
		subDoc := &ParsedDocument{
			Text: parent.Text,
			Sections: []Section{{
				Heading: parent.Heading,
				Body:    parent.Text,
			}},
		}
		subChunks := SplitDocument(subDoc, childOpts)
		for _, subChunk := range subChunks {
			children = append(children, ChildChunk{
				Chunk: Chunk{
					Text:    subChunk.Text,
					Index:   nextChildIndex,
					Heading: parent.Heading,
				},
				ParentIndex: parentIndex,
			})
			nextChildIndex++
		}
	}
	log.Printf("level=info component=parser event=split_hierarchical_complete parents=%d children=%d parent_size=%d child_size=%d overlap=%d",
		len(parents), len(children), normalizeSplitOptions(parentOpts).ChunkSize, normalizeSplitOptions(childOpts).ChunkSize, normalizeSplitOptions(parentOpts).Overlap)
	return &HierarchicalChunks{Parents: parents, Children: children}
}

// SplitText is a convenience wrapper that splits plain text into string chunks.
func SplitText(text string, maxChunkSize int) []string {
	chunks := SplitDocument(&ParsedDocument{Text: text}, SplitOptions{ChunkSize: maxChunkSize})
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	return texts
}

func normalizeSplitOptions(opts SplitOptions) SplitOptions {
	if opts.ChunkSize <= 0 {
		opts.ChunkSize = defaultChunkSize
	}
	if opts.Overlap < 0 {
		opts.Overlap = defaultChunkOverlap
	}
	if opts.Overlap == 0 {
		opts.Overlap = defaultChunkOverlap
	}
	if opts.Overlap >= opts.ChunkSize {
		opts.Overlap = opts.ChunkSize / 5
	}
	return opts
}

func splitRunes(text, heading string, opts SplitOptions, nextIndex *int) []Chunk {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}

	var chunks []Chunk
	for start := 0; start < len(runes); {
		targetEnd := start + opts.ChunkSize
		if targetEnd >= len(runes) {
			targetEnd = len(runes)
		} else {
			relativeEnd := findBreakPoint(string(runes[start:targetEnd]), targetEnd-start)
			if relativeEnd > 0 {
				targetEnd = start + relativeEnd
			}
		}
		if targetEnd <= start {
			targetEnd = min(start+opts.ChunkSize, len(runes))
		}

		chunkText := strings.TrimSpace(string(runes[start:targetEnd]))
		if chunkText != "" {
			chunks = append(chunks, Chunk{
				Text:    chunkText,
				Index:   *nextIndex,
				Heading: heading,
			})
			*nextIndex = *nextIndex + 1
		}
		if targetEnd >= len(runes) {
			break
		}

		nextStart := targetEnd - opts.Overlap
		if nextStart <= start {
			nextStart = start + max(1, opts.ChunkSize-opts.Overlap)
		}
		for nextStart < len(runes) && isLeadingChunkSpace(runes[nextStart]) {
			nextStart++
		}
		start = nextStart
	}
	return chunks
}

func findBreakPoint(text string, idealPos int) int {
	runes := []rune(text)
	if idealPos > len(runes) {
		idealPos = len(runes)
	}
	if idealPos <= 0 {
		return 0
	}

	start := idealPos - maxLookback
	if start < 0 {
		start = 0
	}
	searches := []func(int) (int, bool){
		func(i int) (int, bool) {
			if i > 0 && runes[i-1] == '\n' && runes[i] == '\n' {
				return i + 1, true
			}
			return 0, false
		},
		func(i int) (int, bool) {
			if runes[i] == '。' {
				return i + 1, true
			}
			return 0, false
		},
		func(i int) (int, bool) {
			switch runes[i] {
			case '；', '！', '？':
				return i + 1, true
			default:
				return 0, false
			}
		},
		func(i int) (int, bool) {
			if runes[i] == '.' && (i+1 >= len(runes) || runes[i+1] == ' ' || runes[i+1] == '\n') {
				return i + 1, true
			}
			return 0, false
		},
		func(i int) (int, bool) {
			if runes[i] == '，' {
				return i + 1, true
			}
			return 0, false
		},
	}

	for _, search := range searches {
		for i := idealPos - 1; i >= start; i-- {
			if pos, ok := search(i); ok && pos > 0 {
				return pos
			}
		}
	}
	return idealPos
}

func runeLen(text string) int {
	return len([]rune(text))
}

func isLeadingChunkSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}
