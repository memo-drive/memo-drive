package store

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
)

func TestStoreRejectsSecondActiveTaskAcrossTaskWriters(t *testing.T) {
	cfg := &config.Config{Storage: config.StorageConfig{
		DBPath: filepath.Join(t.TempDir(), "db", "memodrive.db"),
	}}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.DBPath), 0o755); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	db, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	file := &model.File{ID: "file-1", Name: "report.md", Path: "/", StoragePath: "report.md", Status: model.FileStatusFailed}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create File: %v", err)
	}
	if err := db.CreateRetryTask(context.Background(), &model.Task{
		ID: "retry-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusPending,
	}); err != nil {
		t.Fatalf("create retry Task: %v", err)
	}

	err = db.CreateTask(context.Background(), &model.Task{
		ID: "regular-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusPending,
	})
	if !errors.Is(err, ErrTaskAlreadyActive) {
		t.Fatalf("create second active Task error = %v, want ErrTaskAlreadyActive", err)
	}
}

func TestTaskRetryMigrationPreservesExistingTasks(t *testing.T) {
	cfg := &config.Config{Storage: config.StorageConfig{
		DBPath: filepath.Join(t.TempDir(), "db", "memodrive.db"),
	}}
	if err := os.MkdirAll(filepath.Dir(cfg.Storage.DBPath), 0o755); err != nil {
		t.Fatalf("create database directory: %v", err)
	}
	first, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open initial store: %v", err)
	}
	file := &model.File{ID: "legacy-file", Name: "legacy.md", Path: "/", StoragePath: "legacy.md", Status: model.FileStatusFailed}
	if err := first.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create legacy File: %v", err)
	}
	if err := first.CreateTask(context.Background(), &model.Task{
		ID: "legacy-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusFailed, Progress: 100,
	}); err != nil {
		t.Fatalf("create legacy Task: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("close initial store: %v", err)
	}

	raw, err := sql.Open("sqlite3", cfg.Storage.DBPath)
	if err != nil {
		t.Fatalf("open migration database: %v", err)
	}
	if _, err := raw.Exec(`
DROP INDEX IF EXISTS idx_tasks_retry_of;
ALTER TABLE tasks DROP COLUMN retry_of_task_id;
DELETE FROM schema_migrations WHERE id = '021_task_retries';
`); err != nil {
		_ = raw.Close()
		t.Fatalf("simulate pre-021 Task schema: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatalf("close migration database: %v", err)
	}

	reopened, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("migrate pre-021 store: %v", err)
	}
	defer reopened.Close()
	legacy, err := reopened.GetTask(context.Background(), "legacy-task")
	if err != nil {
		t.Fatalf("get preserved legacy Task: %v", err)
	}
	if legacy.Status != model.TaskStatusFailed || legacy.RetryOfTaskID != "" {
		t.Fatalf("unexpected migrated legacy Task: %#v", legacy)
	}
	linked := &model.Task{
		ID: "linked-task", FileID: file.ID, Type: "pipeline", Status: model.TaskStatusPending,
		RetryCount: 1, RetryOfTaskID: legacy.ID,
	}
	if err := reopened.CreateRetryTask(context.Background(), linked); err != nil {
		t.Fatalf("create linked Task after migration: %v", err)
	}
	stored, err := reopened.GetTask(context.Background(), linked.ID)
	if err != nil {
		t.Fatalf("get linked Task: %v", err)
	}
	if stored.RetryOfTaskID != legacy.ID || stored.RetryCount != 1 {
		t.Fatalf("unexpected linked Task after migration: %#v", stored)
	}
}
