package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/llm"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
	"github.com/memodrive/backend/internal/vectordb"
)

type mockEmbedProvider struct {
	failures int
	batches  [][]string
}

func (p *mockEmbedProvider) Name() string {
	return "mock"
}

func (p *mockEmbedProvider) Chat(ctx context.Context, messages []llm.Message) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (p *mockEmbedProvider) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	return "", nil
}

func (p *mockEmbedProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	p.batches = append(p.batches, append([]string(nil), texts...))
	if p.failures > 0 {
		p.failures--
		return nil, errors.New("temporary failure")
	}
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = []float32{float32(len(texts)), float32(i)}
	}
	return embeddings, nil
}

func TestPipelineEnqueueRespectsWorkerLimit(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	cfg.Pipeline.Workers = 1
	cfg.Pipeline.EmbedBatchSize = 1
	provider := newBlockingEmbedProvider()
	service := NewPipelineService(cfg, db, provider, noopVectorStore{}, nil, nil)

	file1 := createPipelineTestFile(t, cfg, db, "file-1", "one.md")
	file2 := createPipelineTestFile(t, cfg, db, "file-2", "two.md")

	task1, err := service.Enqueue(context.Background(), file1)
	if err != nil {
		t.Fatalf("enqueue first file: %v", err)
	}
	provider.waitForEmbed(t)

	task2, err := service.Enqueue(context.Background(), file2)
	if err != nil {
		t.Fatalf("enqueue second file: %v", err)
	}
	defer provider.releaseEmbeds()

	assertTaskStatusEventually(t, service, task1.ID, model.TaskStatusProcessing)
	assertTaskStaysStatus(t, service, task2.ID, model.TaskStatusPending, 150*time.Millisecond)
}

func TestPipelineRequeueRespectsWorkerLimit(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	cfg.Pipeline.Workers = 1
	cfg.Pipeline.EmbedBatchSize = 1
	provider := newBlockingEmbedProvider()
	service := NewPipelineService(cfg, db, provider, noopVectorStore{}, nil, nil)

	file1 := createPipelineTestFile(t, cfg, db, "file-1", "one.md")
	file2 := createPipelineTestFile(t, cfg, db, "file-2", "two.md")
	recoveredTask := &model.Task{
		ID:       "recovered-task",
		FileID:   file2.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusPending,
		Progress: 0,
	}
	if err := db.CreateTask(context.Background(), recoveredTask); err != nil {
		t.Fatalf("create recovered task: %v", err)
	}

	task1, err := service.Enqueue(context.Background(), file1)
	if err != nil {
		t.Fatalf("enqueue first file: %v", err)
	}
	provider.waitForEmbed(t)

	if err := service.Requeue(context.Background(), recoveredTask.ID, file2); err != nil {
		t.Fatalf("requeue recovered task: %v", err)
	}
	defer provider.releaseEmbeds()

	assertTaskStatusEventually(t, service, task1.ID, model.TaskStatusProcessing)
	assertTaskStaysStatus(t, service, recoveredTask.ID, model.TaskStatusPending, 150*time.Millisecond)
}

func TestPipelineShutdownWaitsForAcceptedTasks(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	cfg.Pipeline.Workers = 1
	cfg.Pipeline.EmbedBatchSize = 1
	provider := newBlockingEmbedProvider()
	service := NewPipelineService(cfg, db, provider, noopVectorStore{}, nil, nil)
	file := createPipelineTestFile(t, cfg, db, "file-1", "one.md")

	task, err := service.Enqueue(context.Background(), file)
	if err != nil {
		t.Fatalf("enqueue file: %v", err)
	}
	provider.waitForEmbed(t)

	done := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		done <- service.Shutdown(ctx)
	}()

	select {
	case err := <-done:
		t.Fatalf("Shutdown returned before accepted task completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	provider.releaseEmbeds()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Shutdown returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Shutdown")
	}
	assertTaskStatusEventually(t, service, task.ID, model.TaskStatusDone)
}

func TestPipelineRequeueAfterShutdownMarksTaskFailed(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	service := NewPipelineService(cfg, db, &mockEmbedProvider{}, noopVectorStore{}, nil, nil)
	file := createPipelineTestFile(t, cfg, db, "file-1", "one.md")
	task := &model.Task{
		ID:       "task-1",
		FileID:   file.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusPending,
		Progress: 0,
	}
	if err := db.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if err := service.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown pipeline: %v", err)
	}

	if err := service.Requeue(context.Background(), task.ID, file); err == nil {
		t.Fatal("expected requeue after shutdown to return an error")
	}

	updated, err := service.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Status != model.TaskStatusFailed || updated.Error == nil {
		t.Fatalf("expected failed task with error, got %#v", updated)
	}
}

func TestPipelineTaskPanicMarksTaskFailed(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	service := NewPipelineService(cfg, db, panicEmbedProvider{}, noopVectorStore{}, nil, nil)
	file := createPipelineTestFile(t, cfg, db, "file-1", "one.md")

	task, err := service.Enqueue(context.Background(), file)
	if err != nil {
		t.Fatalf("enqueue file: %v", err)
	}

	assertTaskStatusEventually(t, service, task.ID, model.TaskStatusFailed)
	updated, err := service.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updated.Error == nil {
		t.Fatal("expected failed task to record panic error")
	}
}

func TestPipelineWritesBM25ChunksWithoutLLMProvider(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	service := NewPipelineService(cfg, db, nil, noopVectorStore{}, nil, nil)
	file := createPipelineTestFile(t, cfg, db, "file-1", "one.md")

	task, err := service.Enqueue(context.Background(), file)
	if err != nil {
		t.Fatalf("enqueue file: %v", err)
	}

	assertTaskStatusEventually(t, service, task.ID, model.TaskStatusDone)
	assertFileSearchableViaBM25(t, db, file, "searchable")
}

func TestPipelineWritesBM25ChunksWithoutVectorStore(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	service := NewPipelineService(cfg, db, &mockEmbedProvider{}, nil, nil, nil)
	file := createPipelineTestFile(t, cfg, db, "file-1", "one.md")

	task, err := service.Enqueue(context.Background(), file)
	if err != nil {
		t.Fatalf("enqueue file: %v", err)
	}

	assertTaskStatusEventually(t, service, task.ID, model.TaskStatusDone)
	assertFileSearchableViaBM25(t, db, file, "searchable")
}

type panicEmbedProvider struct{}

func (panicEmbedProvider) Name() string {
	return "panic"
}

func (panicEmbedProvider) Chat(ctx context.Context, messages []llm.Message) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (panicEmbedProvider) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	return "", nil
}

func (panicEmbedProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	panic("embed panic")
}

type blockingEmbedProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newBlockingEmbedProvider() *blockingEmbedProvider {
	return &blockingEmbedProvider{
		started: make(chan struct{}, 10),
		release: make(chan struct{}),
	}
}

func (p *blockingEmbedProvider) Name() string {
	return "blocking"
}

func (p *blockingEmbedProvider) Chat(ctx context.Context, messages []llm.Message) (<-chan string, error) {
	ch := make(chan string)
	close(ch)
	return ch, nil
}

func (p *blockingEmbedProvider) Complete(ctx context.Context, messages []llm.Message) (string, error) {
	return "", nil
}

func (p *blockingEmbedProvider) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	p.started <- struct{}{}
	select {
	case <-p.release:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	embeddings := make([][]float32, len(texts))
	for i := range texts {
		embeddings[i] = []float32{1, float32(i)}
	}
	return embeddings, nil
}

func (p *blockingEmbedProvider) waitForEmbed(t *testing.T) {
	t.Helper()
	select {
	case <-p.started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for embed to start")
	}
}

func (p *blockingEmbedProvider) releaseEmbeds() {
	p.once.Do(func() {
		close(p.release)
	})
}

type noopVectorStore struct{}

func (noopVectorStore) EnsureCollection(ctx context.Context, name string) error {
	return nil
}

func (noopVectorStore) Upsert(ctx context.Context, collection string, ids []string, embeddings [][]float32, documents []string, metadatas []map[string]any) error {
	return nil
}

func (noopVectorStore) Query(ctx context.Context, collection string, queryEmbedding []float32, nResults int) (*vectordb.QueryResult, error) {
	return &vectordb.QueryResult{}, nil
}

func (noopVectorStore) Delete(ctx context.Context, collection string, ids []string) error {
	return nil
}

func newPipelineTestStore(t *testing.T) (*config.Config, *store.Store) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbnails"),
		},
		Pipeline: config.PipelineConfig{
			Workers:         1,
			EmbedBatchSize:  1,
			ParentChunkSize: 256,
			ChildChunkSize:  128,
			ChunkOverlap:    20,
		},
	}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return cfg, db
}

func createPipelineTestFile(t *testing.T, cfg *config.Config, db *store.Store, id, name string) *model.File {
	t.Helper()
	storagePath := filepath.ToSlash(name)
	absPath := filepath.Join(cfg.Storage.Root, filepath.FromSlash(storagePath))
	body := []byte("# Test\n\nThis document has enough text to produce searchable chunks for the indexing pipeline.")
	if err := os.WriteFile(absPath, body, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	file := &model.File{
		ID:          id,
		Name:        name,
		Path:        "/",
		StoragePath: storagePath,
		Size:        int64(len(body)),
		MimeType:    "text/markdown",
		Status:      model.FileStatusUploaded,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	return file
}

func assertFileSearchableViaBM25(t *testing.T, db *store.Store, file *model.File, query string) {
	t.Helper()
	results, err := db.SearchChunksBM25(context.Background(), query, []string{file.ID}, 10)
	if err != nil {
		t.Fatalf("search chunks: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("expected local BM25 chunks to contain %q for %s", query, file.ID)
	}
	if results[0].FileID != file.ID {
		t.Fatalf("expected result for %s, got %#v", file.ID, results[0])
	}
}

func assertTaskStatusEventually(t *testing.T, service *PipelineService, taskID, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		task, err := service.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.Status == want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, err := service.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	t.Fatalf("expected task %s to become %q, got %q", taskID, want, task.Status)
}

func assertTaskStaysStatus(t *testing.T, service *PipelineService, taskID, want string, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		task, err := service.GetTask(context.Background(), taskID)
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if task.Status != want {
			t.Fatalf("expected task %s to stay %q, got %q", taskID, want, task.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
