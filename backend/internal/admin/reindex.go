package admin

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/indexing"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/maintenance"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/parser"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

type ReindexSummary struct {
	Command        string `json:"command"`
	Success        bool   `json:"success"`
	TotalFiles     int    `json:"total_files"`
	ReindexedFiles int    `json:"reindexed_files"`
	FailedFiles    int    `json:"failed_files"`
}

func (m *Manager) ReindexAll(ctx context.Context) (ReindexSummary, error) {
	summary := ReindexSummary{Command: "reindex"}
	writerLock, err := maintenance.AcquireWriterLock(m.cfg.Storage.DBPath)
	if err != nil {
		return summary, err
	}
	defer writerLock.Close()
	if err := m.cfg.EnsureDirs(); err != nil {
		return summary, fmt.Errorf("ensure reindex directories: %w", err)
	}
	db, err := store.Open(ctx, m.cfg)
	if err != nil {
		return summary, fmt.Errorf("open reindex database: %w", err)
	}
	defer db.Close()

	vectorStore := vectordb.NewChroma(m.cfg.LLM.ChromaBaseURL)
	if err := vectorStore.EnsureCollection(ctx, vectordb.DefaultCollection); err != nil {
		return summary, fmt.Errorf("prepare vector index: %w", err)
	}
	files, err := db.ListFilesForReindex(ctx)
	if err != nil {
		return summary, fmt.Errorf("list files for reindex: %w", err)
	}
	summary.TotalFiles = len(files)
	for index := range files {
		if err := prepareFileForReindex(ctx, m.cfg, db, vectorStore, &files[index]); err != nil {
			return summary, err
		}
	}

	pipeline := service.NewPipelineService(
		m.cfg,
		db,
		llm.NewProvider(m.cfg.LLM),
		vectorStore,
		parser.NewOCRRunner(m.cfg.OCR),
		parser.NewTranscriber(m.cfg.Transcribe),
	)
	taskIDs := make([]string, 0, len(files))
	for index := range files {
		task, err := pipeline.Enqueue(ctx, &files[index])
		if err != nil {
			_ = pipeline.Shutdown(ctx)
			return summary, fmt.Errorf("enqueue file %s for reindex: %w", files[index].ID, err)
		}
		taskIDs = append(taskIDs, task.ID)
	}
	if err := pipeline.Shutdown(ctx); err != nil {
		return summary, fmt.Errorf("wait for reindex pipeline: %w", err)
	}
	for _, taskID := range taskIDs {
		task, err := db.GetTask(ctx, taskID)
		if err != nil {
			return summary, fmt.Errorf("read reindex task %s: %w", taskID, err)
		}
		if task.Status == model.TaskStatusDone {
			summary.ReindexedFiles++
		} else {
			summary.FailedFiles++
		}
	}
	if summary.FailedFiles != 0 {
		return summary, fmt.Errorf("reindex failed for %d files", summary.FailedFiles)
	}
	summary.Success = true
	return summary, nil
}

func prepareFileForReindex(ctx context.Context, cfg *config.Config, db *store.Store, vectorStore vectordb.VectorStore, file *model.File) error {
	if file.ChunkCount > 0 {
		if err := vectorStore.Delete(ctx, vectordb.DefaultCollection, indexing.ChunkIDs(file.ID, file.ChunkCount)); err != nil {
			return fmt.Errorf("clear vector index for file %s: %w", file.ID, err)
		}
	}
	metadata, err := db.GetMetadata(ctx, file.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("read metadata for file %s: %w", file.ID, err)
	}
	if err == nil && metadata.ThumbnailPath != nil && *metadata.ThumbnailPath != "" {
		thumbnailPath, pathErr := safeJoin(cfg.Storage.ThumbnailDir, *metadata.ThumbnailPath)
		if pathErr != nil {
			return fmt.Errorf("resolve thumbnail for file %s: %w", file.ID, pathErr)
		}
		if removeErr := os.Remove(thumbnailPath); removeErr != nil && !os.IsNotExist(removeErr) {
			return fmt.Errorf("remove thumbnail for file %s: %w", file.ID, removeErr)
		}
	}
	if err := db.DeleteChunksByFileID(ctx, file.ID); err != nil {
		return fmt.Errorf("clear chunks for file %s: %w", file.ID, err)
	}
	if err := db.DeleteMetadataByFileID(ctx, file.ID); err != nil {
		return fmt.Errorf("clear metadata for file %s: %w", file.ID, err)
	}
	if err := db.UpdateFileContent(ctx, file.ID, file.Size, file.MimeType, model.FileStatusUploaded, 0); err != nil {
		return fmt.Errorf("reset file %s for reindex: %w", file.ID, err)
	}
	file.Status = model.FileStatusUploaded
	file.ChunkCount = 0
	return nil
}
