package service

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestFileMutationReplaceRestoresOldContentWhenDatabaseCommitFails(t *testing.T) {
	cfg, db := newFileMutationTestStore(t)
	existing := &model.File{
		ID:          "existing-file",
		Name:        "report.txt",
		Path:        "/",
		StoragePath: "report.txt",
		Size:        3,
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
		ChunkCount:  4,
	}
	if err := db.CreateFile(context.Background(), existing); err != nil {
		t.Fatalf("create existing File: %v", err)
	}
	finalPath := filepath.Join(cfg.Storage.Root, existing.StoragePath)
	if err := os.WriteFile(finalPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old File content: %v", err)
	}

	raw, err := sql.Open("sqlite3", cfg.Storage.DBPath)
	if err != nil {
		t.Fatalf("open fault injection database: %v", err)
	}
	if _, err := raw.Exec(`
CREATE TRIGGER fail_file_mutation_commit
BEFORE UPDATE OF size ON files
BEGIN
    SELECT RAISE(ABORT, 'forced File Mutation failure');
END;`); err != nil {
		_ = raw.Close()
		t.Fatalf("install fault injection trigger: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close fault injection database: %v", err)
	}

	_, err = NewFileMutationService(cfg, db).Apply(
		context.Background(),
		FileMutationInput{
			Kind: model.FileMutationKindUploadReplace,
			File: &model.File{
				ID:          existing.ID,
				Name:        existing.Name,
				Path:        existing.Path,
				StoragePath: existing.StoragePath,
				Size:        3,
				MimeType:    "text/plain",
				Status:      model.FileStatusUploaded,
				ChunkCount:  1,
			},
			TargetFile: existing,
		},
		func(writer io.Writer) error {
			_, err := writer.Write([]byte("new"))
			return err
		},
	)
	if err == nil {
		t.Fatal("expected forced database commit failure")
	}

	content, readErr := os.ReadFile(finalPath)
	if readErr != nil {
		t.Fatalf("read File after failed replace: %v", readErr)
	}
	if string(content) != "old" {
		t.Fatalf("expected old content after failed replace, got %q", content)
	}
	stored, getErr := db.GetFile(context.Background(), existing.ID)
	if getErr != nil {
		t.Fatalf("get File after failed replace: %v", getErr)
	}
	if stored.Size != existing.Size || stored.Status != existing.Status || stored.ChunkCount != existing.ChunkCount {
		t.Fatalf("expected old metadata after failed replace, got %#v", stored)
	}
}

func TestFileMutationRejectsIncompleteStagedContent(t *testing.T) {
	cfg, db := newFileMutationTestStore(t)
	file := &model.File{
		ID:          "incomplete-file",
		Name:        "report.txt",
		Path:        "/",
		StoragePath: "report.txt",
		Size:        5,
		MimeType:    "text/plain",
		Status:      model.FileStatusUploaded,
		ChunkCount:  1,
	}
	_, err := NewFileMutationService(cfg, db).Apply(
		context.Background(),
		FileMutationInput{
			Kind: model.FileMutationKindUploadCreate,
			File: file,
		},
		func(writer io.Writer) error {
			_, err := writer.Write([]byte("abc"))
			return err
		},
	)
	if err == nil {
		t.Fatal("expected incomplete staged content to be rejected")
	}
	if _, err := db.GetFile(context.Background(), file.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("expected no File record for incomplete content, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Storage.Root, file.StoragePath)); !os.IsNotExist(err) {
		t.Fatalf("expected no published storage object, got %v", err)
	}
}

func TestFileMutationRechecksCapacityAfterStagingBeforeCommit(t *testing.T) {
	cfg, db := newFileMutationTestStore(t)
	cfg.Storage.TempLimitBytes = 100
	file := &model.File{
		ID:          "capacity-file",
		Name:        "capacity.txt",
		Path:        "/",
		StoragePath: "capacity.txt",
		Size:        5,
		MimeType:    "text/plain",
		Status:      model.FileStatusUploaded,
		ChunkCount:  1,
	}

	_, err := NewFileMutationService(cfg, db).Apply(
		context.Background(),
		FileMutationInput{
			Kind: model.FileMutationKindUploadCreate,
			File: file,
		},
		func(writer io.Writer) error {
			if _, err := writer.Write([]byte("12345")); err != nil {
				return err
			}
			cfg.Storage.TempLimitBytes = 4
			return nil
		},
	)
	if !IsInsufficientStorage(err) {
		t.Fatalf("Apply() error = %v, want capacity rejection before commit", err)
	}
	if _, getErr := db.GetFile(context.Background(), file.ID); !errors.Is(getErr, store.ErrNotFound) {
		t.Fatalf("expected no committed File, got %v", getErr)
	}
	if _, statErr := os.Stat(filepath.Join(cfg.Storage.Root, file.StoragePath)); !os.IsNotExist(statErr) {
		t.Fatalf("expected no published File, stat error = %v", statErr)
	}
}

func TestFileMutationCommitsRecoverablePipelineTaskWithFile(t *testing.T) {
	cfg, db := newFileMutationTestStore(t)
	pipeline := NewPipelineService(cfg, db, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := pipeline.Shutdown(ctx); err != nil {
		t.Fatalf("stop pipeline worker: %v", err)
	}

	file := &model.File{
		ID:          "task-file",
		Name:        "task.txt",
		Path:        "/",
		StoragePath: "task.txt",
		Size:        4,
		MimeType:    "text/plain",
		Status:      model.FileStatusUploaded,
	}
	result, err := NewFileMutationService(cfg, db, pipeline).Apply(
		context.Background(),
		FileMutationInput{
			Kind: model.FileMutationKindUploadCreate,
			File: file,
		},
		func(writer io.Writer) error {
			_, err := io.WriteString(writer, "task")
			return err
		},
	)
	if err != nil {
		t.Fatalf("apply File Mutation with stopped pipeline: %v", err)
	}
	if result.File == nil || result.Task == nil {
		t.Fatalf("expected committed File and recoverable Pipeline Task, got %#v", result)
	}
	task, err := db.GetTask(context.Background(), result.Task.ID)
	if err != nil {
		t.Fatalf("get committed Pipeline Task: %v", err)
	}
	if task.FileID != file.ID || task.Status != model.TaskStatusPending {
		t.Fatalf("expected pending Pipeline Task for %s, got %#v", file.ID, task)
	}
	if _, err := db.GetFile(context.Background(), file.ID); err != nil {
		t.Fatalf("get File committed with Pipeline Task: %v", err)
	}
}

func TestFileMutationMarksCommittedPipelineTaskFailedWhenQueueIsFull(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	provider := newBlockingEmbedProvider()
	pipeline := NewPipelineService(cfg, db, provider, noopVectorStore{}, nil, nil)
	t.Cleanup(func() {
		provider.releaseEmbeds()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := pipeline.Shutdown(ctx); err != nil {
			t.Errorf("shutdown pipeline: %v", err)
		}
	})

	active := createPipelineTestFile(t, cfg, db, "active-file", "active.md")
	queued := createPipelineTestFile(t, cfg, db, "queued-file", "queued.md")
	if _, err := pipeline.Enqueue(context.Background(), active); err != nil {
		t.Fatalf("enqueue active File: %v", err)
	}
	provider.waitForEmbed(t)
	if _, err := pipeline.Enqueue(context.Background(), queued); err != nil {
		t.Fatalf("enqueue queued File: %v", err)
	}

	body := []byte("# Queue full\n\nThis File must remain retryable when the pipeline queue is full.")
	file := &model.File{
		ID:          "queue-full-file",
		Name:        "queue-full.md",
		Path:        "/",
		StoragePath: "queue-full.md",
		Size:        int64(len(body)),
		MimeType:    "text/markdown",
		Status:      model.FileStatusUploaded,
	}
	result, err := NewFileMutationService(cfg, db, pipeline).Apply(
		context.Background(),
		FileMutationInput{Kind: model.FileMutationKindUploadCreate, File: file},
		func(writer io.Writer) error {
			_, err := writer.Write(body)
			return err
		},
	)
	if err != nil {
		t.Fatalf("apply File Mutation: %v", err)
	}

	task, err := db.GetTask(context.Background(), result.Task.ID)
	if err != nil {
		t.Fatalf("get committed Pipeline Task: %v", err)
	}
	if task.Status != model.TaskStatusFailed {
		t.Fatalf("Task status = %q, want %q", task.Status, model.TaskStatusFailed)
	}
	storedFile, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get committed File: %v", err)
	}
	if storedFile.Status != model.FileStatusFailed {
		t.Fatalf("File status = %q, want %q", storedFile.Status, model.FileStatusFailed)
	}
}

func TestFileMutationSerializesSameTargetAcrossServiceInstances(t *testing.T) {
	cfg, db := newFileMutationTestStore(t)

	first := NewFileMutationService(cfg, db)
	second := NewFileMutationService(cfg, db)
	firstWriting := make(chan struct{})
	secondWriting := make(chan struct{})
	releaseFirst := make(chan struct{})
	results := make(chan error, 2)

	go func() {
		_, err := first.Apply(context.Background(), FileMutationInput{
			Kind: model.FileMutationKindUploadCreate,
			File: &model.File{
				ID:          "first-file",
				Name:        "report.txt",
				Path:        "/",
				StoragePath: "first-report.txt",
				Size:        5,
				MimeType:    "text/plain",
				Status:      model.FileStatusUploaded,
				ChunkCount:  1,
			},
		}, func(writer io.Writer) error {
			close(firstWriting)
			<-releaseFirst
			_, err := writer.Write([]byte("first"))
			return err
		})
		results <- err
	}()
	<-firstWriting

	go func() {
		_, err := second.Apply(context.Background(), FileMutationInput{
			Kind: model.FileMutationKindUploadCreate,
			File: &model.File{
				ID:          "second-file",
				Name:        "report.txt",
				Path:        "/",
				StoragePath: "second-report.txt",
				Size:        6,
				MimeType:    "text/plain",
				Status:      model.FileStatusUploaded,
				ChunkCount:  1,
			},
		}, func(writer io.Writer) error {
			close(secondWriting)
			_, err := writer.Write([]byte("second"))
			return err
		})
		results <- err
	}()

	overlapped := false
	select {
	case <-secondWriting:
		overlapped = true
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseFirst)
	firstErr := <-results
	secondErr := <-results

	if overlapped {
		t.Fatal("same Target Path mutations entered their write phase concurrently")
	}
	if firstErr != nil {
		t.Fatalf("first mutation failed: %v", firstErr)
	}
	var conflict *FileConflictError
	if !errors.As(secondErr, &conflict) {
		t.Fatalf("expected second mutation path conflict, got %v", secondErr)
	}
}

func newFileMutationTestStore(t *testing.T) (*config.Config, *store.Store) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
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
