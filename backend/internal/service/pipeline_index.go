package service

import (
	"context"
	"log"
	"time"

	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/parser"
	"github.com/memodrive/backend/internal/vectordb"
)

const defaultEmbedBatchSize = 20

// indexParsedDocument runs the full indexing pipeline for a parsed document:
//  1. Build hierarchical index plan (parent + child chunks) from the parsed doc
//  2. If no chunks produced (empty doc), delete any stale chunks and mark ready
//  3. If no LLM provider: persist chunk rows to SQLite only (BM25 search), skip embeddings
//  4. If no vector DB: persist chunk rows only, skip vector upsert
//  5. Otherwise: batch-embed all child chunks, upsert to ChromaDB, persist chunk rows
// The task progresses through: split → embedded → upserted → completed.
func (s *PipelineService) indexParsedDocument(ctx context.Context, taskID string, file *model.File, doc *parser.ParsedDocument, started time.Time) error {
	plan := indexing.BuildDocumentIndexPlan(
		indexing.DocumentRef{ID: file.ID, Name: file.Name},
		doc,
		indexing.DocumentIndexOptions{
			ParentChunkSize: s.parentChunkSize(),
			ChildChunkSize:  s.childChunkSize(),
			ChunkOverlap:    s.cfg.Pipeline.ChunkOverlap,
		},
	)
	file.ChunkCount = plan.ChildCount()
	_ = s.store.UpdateFileChunkCount(ctx, file.ID, file.ChunkCount)
	log.Printf("level=info component=pipeline event=document_split_complete task_id=%s file_id=%s file_name=%q parent_chunks=%d child_chunks=%d truncated=%q parent_chunk_size=%d child_chunk_size=%d chunk_overlap=%d duration_ms=%d",
		taskID, file.ID, file.Name, len(plan.Hierarchy.Parents), plan.ChildCount(), doc.Meta["truncated"], s.parentChunkSize(), s.childChunkSize(), s.cfg.Pipeline.ChunkOverlap, time.Since(started).Milliseconds())
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, pipelineProgressSplit, nil)

	if plan.ChildCount() == 0 {
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
		log.Printf("level=warn component=pipeline event=embed_skipped task_id=%s file_id=%s file_name=%q reason=no_llm_provider chunks=%d", taskID, file.ID, file.Name, plan.ChildCount())
		s.persistIndexChunks(ctx, taskID, file, plan)
		s.markReady(ctx, taskID, file.ID)
		log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=%d indexed=false duration_ms=%d", taskID, file.ID, file.Name, plan.ChildCount(), time.Since(started).Milliseconds())
		return nil
	}
	if s.vectorDB == nil {
		log.Printf("level=warn component=pipeline event=upsert_skipped task_id=%s file_id=%s file_name=%q reason=no_vector_store chunks=%d", taskID, file.ID, file.Name, plan.ChildCount())
		s.persistIndexChunks(ctx, taskID, file, plan)
		s.markReady(ctx, taskID, file.ID)
		log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=%d indexed=false duration_ms=%d", taskID, file.ID, file.Name, plan.ChildCount(), time.Since(started).Milliseconds())
		return nil
	}

	vectorStage := s.vectorIndexStage()
	log.Printf("level=info component=pipeline event=embed_begin task_id=%s file_id=%s file_name=%q provider=%s chunks=%d batch_size=%d",
		taskID, file.ID, file.Name, s.llm.Name(), len(plan.VectorTexts), s.embedBatchSize())
	embeddings, err := vectorStage.EmbedTexts(ctx, plan.VectorTexts)
	if err != nil {
		s.failTask(ctx, taskID, file.ID, err)
		log.Printf("level=error component=pipeline event=embed_failed task_id=%s file_id=%s file_name=%q chunks=%d duration_ms=%d err=%q", taskID, file.ID, file.Name, len(plan.VectorTexts), time.Since(started).Milliseconds(), err)
		return err
	}
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, pipelineProgressEmbedded, nil)
	log.Printf("level=info component=pipeline event=embed_complete task_id=%s file_id=%s file_name=%q chunks=%d dimensions=%d duration_ms=%d",
		taskID, file.ID, file.Name, len(embeddings), embeddingDimensions(embeddings), time.Since(started).Milliseconds())

	upsertStarted := time.Now()
	log.Printf("level=info component=pipeline event=upsert_begin task_id=%s file_id=%s file_name=%q collection=%q chunks=%d",
		taskID, file.ID, file.Name, vectordb.DefaultCollection, plan.ChildCount())
	if err := vectorStage.UpsertPlan(ctx, plan, embeddings); err != nil {
		s.failTask(ctx, taskID, file.ID, err)
		log.Printf("level=error component=pipeline event=upsert_failed task_id=%s file_id=%s file_name=%q collection=%q chunks=%d duration_ms=%d err=%q",
			taskID, file.ID, file.Name, vectordb.DefaultCollection, plan.ChildCount(), time.Since(upsertStarted).Milliseconds(), err)
		return err
	}
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, pipelineProgressUpserted, nil)
	log.Printf("level=info component=pipeline event=upsert_complete task_id=%s file_id=%s file_name=%q collection=%q chunks=%d duration_ms=%d",
		taskID, file.ID, file.Name, vectordb.DefaultCollection, plan.ChildCount(), time.Since(upsertStarted).Milliseconds())

	s.persistIndexChunks(ctx, taskID, file, plan)

	s.markReady(ctx, taskID, file.ID)
	log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=%d indexed=true duration_ms=%d", taskID, file.ID, file.Name, plan.ChildCount(), time.Since(started).Milliseconds())
	return nil
}

func isParsedDocumentEmpty(doc *parser.ParsedDocument) bool {
	return !doc.HasContent()
}

func (s *PipelineService) persistIndexChunks(ctx context.Context, taskID string, file *model.File, plan indexing.DocumentIndexPlan) {
	if s.store == nil {
		return
	}
	parentCount := 0
	if plan.Hierarchy != nil {
		parentCount = len(plan.Hierarchy.Parents)
	}
	if err := s.store.UpsertIndexChunks(ctx, plan.ChunkRecords); err != nil {
		log.Printf("level=warn component=pipeline event=chunk_store_failed task_id=%s file_id=%s file_name=%q chunks=%d err=%q", taskID, file.ID, file.Name, len(plan.ChunkRecords), err)
		return
	}
	log.Printf("level=info component=pipeline event=chunk_store_complete task_id=%s file_id=%s file_name=%q rows=%d parents=%d children=%d",
		taskID, file.ID, file.Name, len(plan.ChunkRecords), parentCount, plan.ChildCount())
}

func (s *PipelineService) vectorIndexStage() indexing.VectorIndexStage {
	return indexing.VectorIndexStage{
		Embedder:   s.llm,
		Store:      s.vectorDB,
		Collection: vectordb.DefaultCollection,
		BatchSize:  s.embedBatchSize(),
	}
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
