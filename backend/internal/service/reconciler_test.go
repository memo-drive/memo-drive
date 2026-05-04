package service

import (
	"context"
	"os"
	"path/filepath"
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
