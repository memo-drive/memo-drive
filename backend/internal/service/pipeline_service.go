package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/parser"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
	"github.com/memodrive/backend/internal/worker"
)

type PipelineService struct {
	cfg         *config.Config
	store       *store.Store
	llm         llm.Provider
	vectorDB    vectordb.VectorStore
	ocr         *parser.OCRRunner
	transcriber parser.Transcriber
	runner      *worker.Pool
}

func NewPipelineService(cfg *config.Config, store *store.Store, llmProvider llm.Provider, vectorDB vectordb.VectorStore, ocr *parser.OCRRunner, transcriber parser.Transcriber) *PipelineService {
	workers := 1
	if cfg != nil && cfg.Pipeline.Workers > 0 {
		workers = cfg.Pipeline.Workers
	}
	return &PipelineService{
		cfg:         cfg,
		store:       store,
		llm:         llmProvider,
		vectorDB:    vectorDB,
		ocr:         ocr,
		transcriber: transcriber,
		runner:      worker.New(workers),
	}
}

func (s *PipelineService) Enqueue(ctx context.Context, file *model.File) (*model.Task, error) {
	started := time.Now()
	task := &model.Task{
		ID:       uuid.NewString(),
		FileID:   file.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusPending,
		Progress: pipelineProgressQueued,
	}
	if err := s.store.CreateTask(ctx, task); err != nil {
		log.Printf("level=error component=pipeline event=enqueue_failed file_id=%s file_name=%q err=%q", file.ID, file.Name, err)
		return nil, err
	}
	log.Printf("level=info component=pipeline event=enqueue task_id=%s file_id=%s file_name=%q mime_type=%q size=%d duration_ms=%d",
		task.ID, file.ID, file.Name, file.MimeType, file.Size, time.Since(started).Milliseconds())
	if err := s.queueTask(task.ID, file); err != nil {
		s.failTask(ctx, task.ID, file.ID, err)
		return nil, err
	}
	return task, nil
}

func (s *PipelineService) GetTask(ctx context.Context, id string) (*model.Task, error) {
	return s.store.GetTask(ctx, id)
}

func (s *PipelineService) run(ctx context.Context, taskID string, file *model.File) {
	started := time.Now()
	log.Printf("level=info component=pipeline event=start task_id=%s file_id=%s file_name=%q mime_type=%q size=%d", taskID, file.ID, file.Name, file.MimeType, file.Size)
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, pipelineProgressStarted, nil)
	_ = s.store.UpdateFileStatus(ctx, file.ID, model.FileStatusProcessing)

	absPath := filepath.Join(s.cfg.Storage.Root, filepath.FromSlash(file.StoragePath))
	if IsMedia(file.MimeType, file.Name) {
		log.Printf("level=info component=pipeline event=media_extract_begin task_id=%s file_id=%s file_name=%q", taskID, file.ID, file.Name)
		meta, thumbnail, err := parser.ExtractMedia(ctx, absPath, file.MimeType, file.ID, s.cfg.Storage.ThumbnailDir)
		if err != nil {
			s.failTask(ctx, taskID, file.ID, err)
			log.Printf("level=error component=pipeline event=media_extract_failed task_id=%s file_id=%s file_name=%q duration_ms=%d err=%q", taskID, file.ID, file.Name, time.Since(started).Milliseconds(), err)
			return
		}
		encoded, _ := json.Marshal(meta)
		var thumb *string
		if thumbnail != "" {
			thumb = &thumbnail
		}
		_ = s.store.UpdateFileChunkCount(ctx, file.ID, 0)
		_ = s.store.UpsertMetadata(ctx, &model.FileMetadata{
			FileID:        file.ID,
			MetaJSON:      string(encoded),
			ThumbnailPath: thumb,
		})
		log.Printf("level=info component=pipeline event=media_extract_complete task_id=%s file_id=%s file_name=%q thumbnail=%t duration_ms=%d", taskID, file.ID, file.Name, thumbnail != "", time.Since(started).Milliseconds())
		_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, pipelineProgressParsed, nil)

		if !s.mediaTextExtractionEnabled() {
			log.Printf("level=info component=pipeline event=media_text_skipped task_id=%s file_id=%s file_name=%q reason=disabled", taskID, file.ID, file.Name)
			s.markReady(ctx, taskID, file.ID)
			log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=0 indexed=false duration_ms=%d", taskID, file.ID, file.Name, time.Since(started).Milliseconds())
			return
		}
		if s.cfg.Pipeline.SkipLarge > 0 && file.Size > s.cfg.Pipeline.SkipLarge {
			log.Printf("level=warn component=pipeline event=media_text_skipped task_id=%s file_id=%s file_name=%q reason=too_large size=%d max_size=%d", taskID, file.ID, file.Name, file.Size, s.cfg.Pipeline.SkipLarge)
			s.markReady(ctx, taskID, file.ID)
			return
		}
		doc, err := s.extractMediaText(ctx, file)
		if err != nil {
			log.Printf("level=warn component=pipeline event=media_text_failed task_id=%s file_id=%s file_name=%q err=%q", taskID, file.ID, file.Name, err)
			s.markReady(ctx, taskID, file.ID)
			return
		}
		if isParsedDocumentEmpty(doc) {
			log.Printf("level=info component=pipeline event=media_text_empty_skip task_id=%s file_id=%s file_name=%q", taskID, file.ID, file.Name)
			s.markReady(ctx, taskID, file.ID)
			log.Printf("level=info component=pipeline event=complete task_id=%s file_id=%s file_name=%q status=ready chunks=0 indexed=false duration_ms=%d", taskID, file.ID, file.Name, time.Since(started).Milliseconds())
			return
		}
		if err := s.indexParsedDocument(ctx, taskID, file, doc, started); err != nil {
			return
		}
		return
	}

	log.Printf("level=info component=pipeline event=document_parse_begin task_id=%s file_id=%s file_name=%q mime_type=%q", taskID, file.ID, file.Name, file.MimeType)
	doc, err := parser.Parse(absPath, file.MimeType)
	if err != nil {
		if errors.Is(err, parser.ErrUnsupportedFormat) {
			log.Printf("level=info component=pipeline event=document_parse_skipped task_id=%s file_id=%s file_name=%q mime_type=%q reason=unsupported err=%q", taskID, file.ID, file.Name, file.MimeType, err)
			_ = s.store.UpdateFileChunkCount(ctx, file.ID, 0)
			s.markReady(ctx, taskID, file.ID)
			return
		}
		s.failTask(ctx, taskID, file.ID, err)
		log.Printf("level=error component=pipeline event=document_parse_failed task_id=%s file_id=%s file_name=%q duration_ms=%d err=%q", taskID, file.ID, file.Name, time.Since(started).Milliseconds(), err)
		return
	}
	_ = s.store.UpdateTask(ctx, taskID, model.TaskStatusProcessing, pipelineProgressParsed, nil)
	log.Printf("level=info component=pipeline event=document_parse_complete task_id=%s file_id=%s file_name=%q chars=%d sections=%d title=%q duration_ms=%d",
		taskID, file.ID, file.Name, len([]rune(doc.Text)), len(doc.Sections), doc.Title, time.Since(started).Milliseconds())
	if doc.Meta == nil {
		doc.Meta = map[string]string{}
	}
	if doc.Meta["source"] == "" {
		doc.Meta["source"] = "document"
	}
	_ = s.indexParsedDocument(ctx, taskID, file, doc, started)
}
