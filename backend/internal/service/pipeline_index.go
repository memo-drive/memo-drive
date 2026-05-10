package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/parser"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

const defaultEmbedBatchSize = 20

func (s *PipelineService) indexParsedDocument(ctx context.Context, taskID string, file *model.File, doc *parser.ParsedDocument, started time.Time) error {
	hierarchy := parser.SplitDocumentHierarchical(doc, s.parentChunkSize(), s.childChunkSize(), s.cfg.Pipeline.ChunkOverlap)
	chunks := childChunks(hierarchy.Children)
	file.ChunkCount = len(chunks)
	_ = s.store.UpdateFileChunkCount(ctx, file.ID, file.ChunkCount)
	log.Printf("level=info component=pipeline event=document_split_complete task_id=%s file_id=%s file_name=%q parent_chunks=%d child_chunks=%d truncated=%q parent_chunk_size=%d child_chunk_size=%d chunk_overlap=%d duration_ms=%d",
		taskID, file.ID, file.Name, len(hierarchy.Parents), len(chunks), doc.Meta["truncated"], s.parentChunkSize(), s.childChunkSize(), s.cfg.Pipeline.ChunkOverlap, time.Since(started).Milliseconds())
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, 45, nil)

	if len(chunks) == 0 {
		if s.store != nil {
			if err := s.store.DeleteChunksByFileID(ctx, file.ID); err != nil {
				log.Printf("level=warn component=pipeline event=chunk_store_delete_failed task_id=%s file_id=%s file_name=%q err=%q", taskID, file.ID, file.Name, err)
			}
		}
		log.Printf("level=info component=pipeline event=document_empty_skip task_id=%s file_id=%s file_name=%q reason=no_chunks duration_ms=%d", taskID, file.ID, file.Name, time.Since(started).Milliseconds())
		s.markReady(ctx, taskID, file.ID)
		log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=0 duration_ms=%d", taskID, file.ID, file.Name, time.Since(started).Milliseconds())
		return nil
	}
	if s.llm == nil {
		log.Printf("level=warn component=pipeline event=embed_skipped task_id=%s file_id=%s file_name=%q reason=no_llm_provider chunks=%d", taskID, file.ID, file.Name, len(chunks))
		s.markReady(ctx, taskID, file.ID)
		log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=%d indexed=false duration_ms=%d", taskID, file.ID, file.Name, len(chunks), time.Since(started).Milliseconds())
		return nil
	}
	if s.vectorDB == nil {
		log.Printf("level=warn component=pipeline event=upsert_skipped task_id=%s file_id=%s file_name=%q reason=no_vector_store chunks=%d", taskID, file.ID, file.Name, len(chunks))
		s.markReady(ctx, taskID, file.ID)
		log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=%d indexed=false duration_ms=%d", taskID, file.ID, file.Name, len(chunks), time.Since(started).Milliseconds())
		return nil
	}

	texts := make([]string, len(chunks))
	for i, chunk := range chunks {
		texts[i] = textWithHeading(chunk.Heading, chunk.Text)
	}

	log.Printf("level=info component=pipeline event=embed_begin task_id=%s file_id=%s file_name=%q provider=%s chunks=%d batch_size=%d",
		taskID, file.ID, file.Name, s.llm.Name(), len(texts), s.embedBatchSize())
	embeddings, err := s.batchEmbed(ctx, texts)
	if err != nil {
		s.failTask(ctx, taskID, file.ID, err)
		log.Printf("level=error component=pipeline event=embed_failed task_id=%s file_id=%s file_name=%q chunks=%d duration_ms=%d err=%q", taskID, file.ID, file.Name, len(texts), time.Since(started).Milliseconds(), err)
		return err
	}
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, 75, nil)
	log.Printf("level=info component=pipeline event=embed_complete task_id=%s file_id=%s file_name=%q chunks=%d dimensions=%d duration_ms=%d",
		taskID, file.ID, file.Name, len(embeddings), embeddingDimensions(embeddings), time.Since(started).Milliseconds())

	ids := make([]string, len(chunks))
	metadatas := make([]map[string]any, len(chunks))
	source := documentSource(doc)
	for i, chunk := range chunks {
		ids[i] = vectordb.ChunkID(file.ID, chunk.Index)
		parentID := parentIDForChild(file.ID, hierarchy.Children[i])
		metadatas[i] = map[string]any{
			"file_id":                      file.ID,
			"file_name":                    file.Name,
			"heading":                      chunk.Heading,
			"chunk_index":                  chunk.Index,
			"source":                       source,
			vectordb.MetadataParentChunkID: parentID,
		}
	}

	upsertStarted := time.Now()
	log.Printf("level=info component=pipeline event=upsert_begin task_id=%s file_id=%s file_name=%q collection=%q chunks=%d",
		taskID, file.ID, file.Name, vectordb.DefaultCollection, len(chunks))
	if err := s.vectorDB.Upsert(ctx, vectordb.DefaultCollection, ids, embeddings, texts, metadatas); err != nil {
		s.failTask(ctx, taskID, file.ID, err)
		log.Printf("level=error component=pipeline event=upsert_failed task_id=%s file_id=%s file_name=%q collection=%q chunks=%d duration_ms=%d err=%q",
			taskID, file.ID, file.Name, vectordb.DefaultCollection, len(chunks), time.Since(upsertStarted).Milliseconds(), err)
		return err
	}
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, 90, nil)
	log.Printf("level=info component=pipeline event=upsert_complete task_id=%s file_id=%s file_name=%q collection=%q chunks=%d duration_ms=%d",
		taskID, file.ID, file.Name, vectordb.DefaultCollection, len(chunks), time.Since(upsertStarted).Milliseconds())

	if s.store != nil {
		chunkRows := chunkRowsForIndex(file, hierarchy, texts)
		if err := s.store.UpsertChunks(ctx, chunkRows); err != nil {
			log.Printf("level=warn component=pipeline event=chunk_store_failed task_id=%s file_id=%s file_name=%q chunks=%d err=%q", taskID, file.ID, file.Name, len(chunkRows), err)
		} else {
			log.Printf("level=info component=pipeline event=chunk_store_complete task_id=%s file_id=%s file_name=%q rows=%d parents=%d children=%d",
				taskID, file.ID, file.Name, len(chunkRows), len(hierarchy.Parents), len(hierarchy.Children))
		}
	}

	s.markReady(ctx, taskID, file.ID)
	log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=%d indexed=true duration_ms=%d", taskID, file.ID, file.Name, len(chunks), time.Since(started).Milliseconds())
	return nil
}

func isParsedDocumentEmpty(doc *parser.ParsedDocument) bool {
	return doc == nil || strings.TrimSpace(doc.Text) == ""
}

func documentSource(doc *parser.ParsedDocument) string {
	if doc != nil && doc.Meta != nil {
		if source := strings.TrimSpace(doc.Meta["source"]); source != "" {
			return source
		}
	}
	return "document"
}

func (s *PipelineService) batchEmbed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	if s.llm == nil {
		return nil, errors.New("llm provider is not configured")
	}
	batchSize := s.embedBatchSize()
	totalBatches := (len(texts) + batchSize - 1) / batchSize
	embeddings := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchNo := start/batchSize + 1
		batch := texts[start:end]
		var batchEmbeddings [][]float32
		var lastErr error
		for attempt := 1; attempt <= 2; attempt++ {
			batchStarted := time.Now()
			result, err := s.llm.Embed(ctx, batch)
			if err == nil && len(result) != len(batch) {
				err = fmt.Errorf("embedding count mismatch: got %d, want %d", len(result), len(batch))
			}
			if err == nil {
				batchEmbeddings = result
				log.Printf("level=info component=pipeline event=embed_batch batch=%d/%d inputs=%d attempt=%d provider=%s duration_ms=%d",
					batchNo, totalBatches, len(batch), attempt, s.llm.Name(), time.Since(batchStarted).Milliseconds())
				break
			}
			lastErr = err
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt == 1 {
				log.Printf("level=warn component=pipeline event=embed_batch_retry batch=%d/%d inputs=%d provider=%s err=%q",
					batchNo, totalBatches, len(batch), s.llm.Name(), err)
				continue
			}
		}
		if batchEmbeddings == nil {
			return nil, fmt.Errorf("embed batch %d/%d failed: %w", batchNo, totalBatches, lastErr)
		}
		embeddings = append(embeddings, batchEmbeddings...)
	}
	return embeddings, nil
}

func (s *PipelineService) embedBatchSize() int {
	if s == nil || s.cfg == nil || s.cfg.Pipeline.EmbedBatchSize <= 0 {
		return defaultEmbedBatchSize
	}
	return s.cfg.Pipeline.EmbedBatchSize
}

func (s *PipelineService) parentChunkSize() int {
	if s == nil || s.cfg == nil || s.cfg.Pipeline.ParentChunkSize <= 0 {
		return 1024
	}
	return s.cfg.Pipeline.ParentChunkSize
}

func (s *PipelineService) childChunkSize() int {
	if s == nil || s.cfg == nil || s.cfg.Pipeline.ChildChunkSize <= 0 {
		return 256
	}
	return s.cfg.Pipeline.ChildChunkSize
}

func embeddingDimensions(embeddings [][]float32) int {
	if len(embeddings) == 0 {
		return 0
	}
	return len(embeddings[0])
}

func childChunks(children []parser.ChildChunk) []parser.Chunk {
	chunks := make([]parser.Chunk, 0, len(children))
	for _, child := range children {
		chunks = append(chunks, child.Chunk)
	}
	return chunks
}

func chunkRowsForIndex(file *model.File, hierarchy *parser.HierarchicalChunks, childTexts []string) []store.ChunkRow {
	if file == nil || hierarchy == nil {
		return nil
	}
	rows := make([]store.ChunkRow, 0, len(hierarchy.Parents)+len(hierarchy.Children))
	for _, parent := range hierarchy.Parents {
		rows = append(rows, store.ChunkRow{
			ID:         vectordb.ParentChunkID(file.ID, parent.Index),
			FileID:     file.ID,
			FileName:   file.Name,
			Heading:    parent.Heading,
			ChunkIndex: parent.Index,
			Text:       textWithHeading(parent.Heading, parent.Text),
			IsParent:   true,
		})
	}
	for i, child := range hierarchy.Children {
		text := textWithHeading(child.Heading, child.Text)
		if i < len(childTexts) && strings.TrimSpace(childTexts[i]) != "" {
			text = childTexts[i]
		}
		rows = append(rows, store.ChunkRow{
			ID:            vectordb.ChunkID(file.ID, child.Index),
			FileID:        file.ID,
			FileName:      file.Name,
			Heading:       child.Heading,
			ChunkIndex:    child.Index,
			Text:          text,
			ParentChunkID: parentIDForChild(file.ID, child),
			IsParent:      false,
		})
	}
	return rows
}

func parentIDForChild(fileID string, child parser.ChildChunk) string {
	if child.ParentIndex < 0 {
		return ""
	}
	return vectordb.ParentChunkID(fileID, child.ParentIndex)
}

func textWithHeading(heading, text string) string {
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
