package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestFolderCopyRecordsCompletedRecoverableOperation(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:               filepath.Join(root, "files"),
		DBPath:             filepath.Join(root, "db", "memodrive.db"),
		TempDir:            filepath.Join(root, "tmp"),
		ThumbnailDir:       filepath.Join(root, "thumbs"),
		FolderCopyMaxNodes: 100,
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	for _, file := range []*model.File{
		{ID: "source", Name: "Source", Path: "/", StoragePath: "Source", IsDir: true, Status: model.FileStatusReady},
		{ID: "child", Name: "memo.txt", Path: "/Source", StoragePath: "Source/memo.txt", Size: 4, MimeType: "text/plain", Status: model.FileStatusReady},
	} {
		abs := filepath.Join(cfg.Storage.Root, filepath.FromSlash(file.StoragePath))
		if file.IsDir {
			err = os.MkdirAll(abs, 0o755)
		} else {
			err = os.WriteFile(abs, []byte("memo"), 0o644)
		}
		if err != nil {
			t.Fatalf("create source storage: %v", err)
		}
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create source metadata: %v", err)
		}
	}

	result, err := NewFileService(cfg, db, nil).Copy(context.Background(), "source", FileCopyInput{
		Path: "/", Name: "Source-copy", ConflictPolicy: ConflictReject,
	})
	if err != nil {
		t.Fatalf("copy Folder: %v", err)
	}
	operations, err := db.ListFileCopyOperationsByState(context.Background(), model.FileCopyOperationStateCompleted)
	if err != nil {
		t.Fatalf("list completed Folder Copy operations: %v", err)
	}
	if len(operations) != 1 || operations[0].SourceID != "source" || operations[0].RootFileID != result.File.ID {
		t.Fatalf("completed Folder Copy operations = %#v, root = %#v", operations, result.File)
	}
}

func TestFileCopyCreatesIndependentPipelineTaskForCopiedFile(t *testing.T) {
	cfg, db := newPipelineTestStore(t)
	source := createPipelineTestFile(t, cfg, db, "source-copy-task", "source-copy-task.md")
	pipeline := NewPipelineService(cfg, db, nil, nil, nil, nil)
	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = pipeline.Shutdown(shutdownCtx)
	files := NewFileService(cfg, db, nil)
	files.SetPipeline(pipeline)

	result, err := files.Copy(context.Background(), source.ID, FileCopyInput{
		Path: "/", Name: "copied-task.md", ConflictPolicy: ConflictReject,
	})
	if err != nil {
		t.Fatalf("copy File with Pipeline: %v", err)
	}
	if result.TaskID == "" {
		t.Fatal("copied File has no independent Pipeline Task ID")
	}
	task, err := pipeline.GetTask(context.Background(), result.TaskID)
	if err != nil {
		t.Fatalf("get copied File Pipeline Task: %v", err)
	}
	if task.FileID != result.File.ID || task.FileID == source.ID {
		t.Fatalf("copied File Pipeline Task = %#v, source = %q, copy = %q", task, source.ID, result.File.ID)
	}
}
