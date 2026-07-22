package service

import (
	"bytes"
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestReconcilerPeriodicSweepFailsStuckTasks(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Janitor.MaxTaskAge = -time.Second
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))

	file := &model.File{
		ID:          "file-1",
		Name:        "sample.md",
		Path:        "/",
		StoragePath: "sample.md",
		MimeType:    "text/markdown",
		Status:      model.FileStatusProcessing,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	task := &model.Task{
		ID:       "task-1",
		FileID:   file.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusProcessing,
		Progress: 45,
	}
	if err := db.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}

	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	updatedTask, err := db.GetTask(context.Background(), task.ID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if updatedTask.Status != model.TaskStatusFailed || updatedTask.Error == nil {
		t.Fatalf("expected failed task with error, got %#v", updatedTask)
	}
	updatedFile, err := db.GetFile(context.Background(), file.ID)
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	if updatedFile.Status != model.FileStatusFailed {
		t.Fatalf("expected failed file, got %q", updatedFile.Status)
	}
}

func TestReconcilerPeriodicSweepCleansWebDAVTemp(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	webDAVTempDir := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(webDAVTempDir, 0o755); err != nil {
		t.Fatalf("create webdav temp dir: %v", err)
	}
	expired := filepath.Join(webDAVTempDir, "expired.upload")
	if err := writeSmallFile(expired); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatalf("age temp file: %v", err)
	}

	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	if exists(expired) {
		t.Fatal("expected periodic sweep to remove expired WebDAV temp file")
	}
}

func TestReconcilerRecoverOnBootRequeuesThroughPipeline(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	cfg.Pipeline.Workers = 1
	cfg.Pipeline.EmbedBatchSize = 1
	provider := newBlockingEmbedProvider()
	pipeline := NewPipelineService(cfg, db, provider, noopVectorStore{}, nil, nil)
	reconciler := NewReconciler(cfg, db, pipeline, NewFileService(cfg, db, nil))

	activeFile := createPipelineTestFile(t, cfg, db, "active-file", "active.md")
	recoveredFile := createPipelineTestFile(t, cfg, db, "recovered-file", "recovered.md")
	activeTask, err := pipeline.Enqueue(context.Background(), activeFile)
	if err != nil {
		t.Fatalf("enqueue active file: %v", err)
	}
	provider.waitForEmbed(t)
	if err := db.UpdateTask(context.Background(), activeTask.ID, model.TaskStatusDone, 100, nil); err != nil {
		t.Fatalf("hide active task from recovery query: %v", err)
	}

	recoveredTask := &model.Task{
		ID:       "recovered-task",
		FileID:   recoveredFile.ID,
		Type:     "pipeline",
		Status:   model.TaskStatusPending,
		Progress: 0,
	}
	if err := db.CreateTask(context.Background(), recoveredTask); err != nil {
		t.Fatalf("create recovered task: %v", err)
	}

	if err := reconciler.RecoverOnBoot(context.Background()); err != nil {
		t.Fatalf("RecoverOnBoot returned error: %v", err)
	}
	defer provider.releaseEmbeds()

	updated, err := db.GetTask(context.Background(), recoveredTask.ID)
	if err != nil {
		t.Fatalf("get recovered task: %v", err)
	}
	if updated.RetryCount != 1 {
		t.Fatalf("expected recovered task retry count 1, got %d", updated.RetryCount)
	}
	assertTaskStaysStatus(t, pipeline, recoveredTask.ID, model.TaskStatusPending, 150*time.Millisecond)
}

func TestReconcilerSweepThumbnailsRemovesOrphans(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	orphan := filepath.Join(cfg.Storage.ThumbnailDir, "missing-file.jpg")
	if err := writeSmallFile(orphan); err != nil {
		t.Fatalf("write thumbnail: %v", err)
	}

	removed, err := reconciler.SweepThumbnails(context.Background())
	if err != nil {
		t.Fatalf("SweepThumbnails returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one thumbnail removed, got %d", removed)
	}
	if exists(orphan) {
		t.Fatal("expected orphan thumbnail to be removed")
	}
}

func TestReconcilerSweepWebDAVTempRemovesExpiredFiles(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	webDAVTempDir := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(webDAVTempDir, 0o755); err != nil {
		t.Fatalf("create webdav temp dir: %v", err)
	}
	expired := filepath.Join(webDAVTempDir, "expired.upload")
	if err := writeSmallFile(expired); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(expired, old, old); err != nil {
		t.Fatalf("age temp file: %v", err)
	}

	removed, err := reconciler.SweepWebDAVTemp(context.Background())
	if err != nil {
		t.Fatalf("SweepWebDAVTemp returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected one expired WebDAV temp file removed, got %d", removed)
	}
	if exists(expired) {
		t.Fatal("expected expired WebDAV temp file to be removed")
	}
}

func TestReconcilerSweepWebDAVTempKeepsUnexpiredFiles(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	reconciler := NewReconciler(cfg, db, nil, NewFileService(cfg, db, nil))
	webDAVTempDir := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := os.MkdirAll(webDAVTempDir, 0o755); err != nil {
		t.Fatalf("create webdav temp dir: %v", err)
	}
	active := filepath.Join(webDAVTempDir, "active.upload")
	if err := writeSmallFile(active); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	recent := time.Now().Add(-5 * time.Minute)
	if err := os.Chtimes(active, recent, recent); err != nil {
		t.Fatalf("age temp file: %v", err)
	}

	removed, err := reconciler.SweepWebDAVTemp(context.Background())
	if err != nil {
		t.Fatalf("SweepWebDAVTemp returned error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("expected no active WebDAV temp files removed, got %d", removed)
	}
	if !exists(active) {
		t.Fatal("expected active WebDAV temp file to remain")
	}
}

func TestReconcilerPeriodicSweepLogsWebDAVTempFailureAndContinues(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Storage.UploadTTL = time.Hour
	cfg.Trash.RetentionDays = 0
	files := NewFileService(cfg, db, nil)
	reconciler := NewReconciler(cfg, db, nil, files)
	brokenWebDAVTemp := filepath.Join(cfg.Storage.TempDir, "webdav")
	if err := writeSmallFile(brokenWebDAVTemp); err != nil {
		t.Fatalf("write broken webdav temp path: %v", err)
	}
	file := &model.File{
		ID:          "trash-after-webdav-temp-failure",
		Name:        "expired.txt",
		Path:        "/",
		StoragePath: "expired.txt",
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := writeSmallFile(filepath.Join(cfg.Storage.Root, file.StoragePath)); err != nil {
		t.Fatalf("write storage file: %v", err)
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := files.SoftDelete(context.Background(), file.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	var logs bytes.Buffer
	previousOutput := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(previousOutput)
	})

	if err := reconciler.PeriodicSweep(context.Background()); err != nil {
		t.Fatalf("PeriodicSweep returned error: %v", err)
	}
	if !strings.Contains(logs.String(), "event=webdav_temp_sweep_failed") {
		t.Fatalf("expected WebDAV temp sweep failure log, got %q", logs.String())
	}
	if _, err := db.GetFileIncludeDeleted(context.Background(), file.ID); err == nil {
		t.Fatal("expected trash sweep to continue and purge expired file")
	}
}

func TestReconcilerSweepTrashPurgesExpiredItems(t *testing.T) {
	cfg, db := newReconcilerTestStore(t)
	cfg.Trash.AutoPurgeEnabled = true
	cfg.Trash.RetentionDays = 0
	files := NewFileService(cfg, db, nil)
	reconciler := NewReconciler(cfg, db, nil, files)
	file := &model.File{
		ID:          "trash-file",
		Name:        "expired.txt",
		Path:        "/",
		StoragePath: "expired.txt",
		MimeType:    "text/plain",
		Status:      model.FileStatusReady,
	}
	if err := writeSmallFile(filepath.Join(cfg.Storage.Root, file.StoragePath)); err != nil {
		t.Fatalf("write storage file: %v", err)
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if err := files.SoftDelete(context.Background(), file.ID); err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	purged, err := reconciler.SweepTrash(context.Background())
	if err != nil {
		t.Fatalf("SweepTrash returned error: %v", err)
	}
	if purged != 1 {
		t.Fatalf("expected one trash item purged, got %d", purged)
	}
	if exists(filepath.Join(cfg.Storage.Root, file.StoragePath)) {
		t.Fatal("expected storage file to be removed")
	}
	if _, err := db.GetFileIncludeDeleted(context.Background(), file.ID); err == nil {
		t.Fatal("expected DB row to be purged")
	}
}

func newReconcilerTestStore(t *testing.T) (*config.Config, *store.Store) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbnails"),
		},
		Janitor: config.JanitorConfig{
			MaxTaskAge: time.Minute,
		},
		Trash: config.TrashConfig{
			AutoPurgeEnabled: true,
			RetentionDays:    30,
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

func writeSmallFile(path string) error {
	return os.WriteFile(path, []byte("x"), 0o644)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
