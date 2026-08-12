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

var (
	ErrInvalidTaskStatus = errors.New("invalid task status")
	ErrPipelineQueueFull = worker.ErrFull
)

type TaskNotFailedError struct {
	TaskID string
	Status string
}

type TaskAlreadyActiveError struct {
	FileID string
}

type TaskFileInTrashError struct {
	FileID string
}

func (e *TaskFileInTrashError) Error() string {
	return "File is in Trash"
}

func (e *TaskAlreadyActiveError) Error() string {
	return "File already has an active pipeline Task"
}

func (e *TaskNotFailedError) Error() string {
	return "only failed pipeline Tasks can be retried"
}

// PipelineService orchestrates the document processing pipeline:
// parse -> split -> embed -> index. It uses a worker pool for asynchronous execution.
type PipelineService struct {
	cfg         *config.Config
	store       *store.Store
	llm         llm.Provider
	vectorDB    vectordb.VectorStore
	ocr         *parser.OCRRunner
	transcriber parser.Transcriber
	runner      *worker.Pool
}

// NewPipelineService creates a new PipelineService with the configured number of workers.
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

// Enqueue creates a pipeline task for the given file and submits it to the worker pool.
func (s *PipelineService) Enqueue(ctx context.Context, file *model.File) (*model.Task, error) {
	task := newPipelineTask(file.ID)
	if err := s.store.CreateTask(ctx, task); err != nil {
		log.Printf("level=error component=pipeline event=enqueue_failed file_id=%s file_name=%q err=%q", file.ID, file.Name, err)
		return nil, err
	}
	if err := s.enqueuePersisted(ctx, task, file); err != nil {
		return nil, err
	}
	return task, nil
}

func newPipelineTask(fileID string) *model.Task {
	return &model.Task{
		ID:       uuid.NewString(),
		FileID:   fileID,
		Type:     "pipeline",
		Status:   model.TaskStatusPending,
		Progress: pipelineProgressQueued,
	}
}

func (s *PipelineService) enqueuePersisted(ctx context.Context, task *model.Task, file *model.File) error {
	if err := s.submitPersisted(task, file); err != nil {
		s.failTask(ctx, task.ID, file.ID, err)
		return err
	}
	return nil
}

func (s *PipelineService) submitPersisted(task *model.Task, file *model.File) error {
	started := time.Now()
	log.Printf("level=info component=pipeline event=enqueue task_id=%s file_id=%s file_name=%q mime_type=%q size=%d duration_ms=%d",
		task.ID, file.ID, file.Name, file.MimeType, file.Size, time.Since(started).Milliseconds())
	return s.queueTask(task.ID, file)
}

// GetTask retrieves a pipeline task by ID.
func (s *PipelineService) GetTask(ctx context.Context, id string) (*model.Task, error) {
	task, err := s.store.GetTask(ctx, id)
	if err != nil {
		return nil, err
	}
	public := s.publicTask(*task)
	return &public, nil
}

// RetryTask creates a new pipeline Task linked to an earlier attempt.
func (s *PipelineService) RetryTask(ctx context.Context, taskID string) (*model.Task, error) {
	previous, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if previous.Status != model.TaskStatusFailed {
		return nil, &TaskNotFailedError{TaskID: previous.ID, Status: previous.Status}
	}
	file, err := s.store.GetFileIncludeDeleted(ctx, previous.FileID)
	if err != nil {
		return nil, err
	}
	if file.DeletedAt != nil {
		return nil, &TaskFileInTrashError{FileID: file.ID}
	}
	task := newPipelineTask(file.ID)
	task.RetryCount = previous.RetryCount + 1
	task.RetryOfTaskID = previous.ID
	if err := s.store.CreateRetryTask(ctx, task); err != nil {
		if errors.Is(err, store.ErrTaskAlreadyActive) {
			return nil, &TaskAlreadyActiveError{FileID: file.ID}
		}
		return nil, err
	}
	if err := s.clearRetryArtifacts(ctx, file); err != nil {
		s.failTask(ctx, task.ID, file.ID, err)
		return nil, err
	}
	if err := s.enqueuePersisted(ctx, task, file); err != nil {
		return nil, err
	}
	return task, nil
}

// ListTasks returns pipeline Task audit records with their current File summary.
func (s *PipelineService) ListTasks(ctx context.Context, status, fileID, cursor string, limit int) (*model.TaskListPage, error) {
	switch status {
	case "", model.TaskStatusPending, model.TaskStatusProcessing, model.TaskStatusDone, model.TaskStatusFailed:
	default:
		return nil, ErrInvalidTaskStatus
	}
	items, nextCursor, hasMore, err := s.store.ListTaskItems(ctx, store.TaskListFilter{
		Status: status,
		FileID: fileID,
		Cursor: cursor,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Task = s.publicTask(items[i].Task)
	}
	return &model.TaskListPage{Items: items, NextCursor: nextCursor, HasMore: hasMore}, nil
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
