package service

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"testing"

	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/store"
)

func TestFolderZIPWriteStopsWhenContextIsCanceled(t *testing.T) {
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	cfg := &config.Config{Storage: config.StorageConfig{
		Root:                          storageRoot,
		DBPath:                        filepath.Join(root, "db", "memodrive.db"),
		TempDir:                       filepath.Join(root, "tmp"),
		ThumbnailDir:                  filepath.Join(root, "thumbs"),
		FolderZIPMaxNodes:             100,
		FolderZIPMaxUncompressedBytes: 2 * 1024 * 1024,
	}}
	if err := cfg.EnsureDirs(); err != nil {
		t.Fatalf("ensure dirs: %v", err)
	}
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	content := make([]byte, 1024*1024)
	if _, err := rand.New(rand.NewSource(1)).Read(content); err != nil {
		t.Fatalf("prepare deterministic content: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(storageRoot, "Folder"), 0o755); err != nil {
		t.Fatalf("mkdir Folder: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, "Folder", "large.bin"), content, 0o644); err != nil {
		t.Fatalf("write large File: %v", err)
	}
	for _, file := range []*model.File{
		{ID: "folder", Name: "Folder", Path: "/", StoragePath: "Folder", IsDir: true, Status: model.FileStatusReady},
		{ID: "large", Name: "large.bin", Path: "/Folder", StoragePath: "Folder/large.bin", Size: int64(len(content)), MimeType: "application/octet-stream", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create %s: %v", file.ID, err)
		}
	}

	archive, err := NewFileService(cfg, db, nil).PrepareFolderZIP(context.Background(), "folder")
	if err != nil {
		t.Fatalf("prepare Folder ZIP: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	output := &cancelOnFirstWrite{cancel: cancel}
	err = archive.Write(ctx, output)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Folder ZIP cancellation error = %v, want context.Canceled", err)
	}
}

type cancelOnFirstWrite struct {
	cancel   context.CancelFunc
	canceled bool
}

func (w *cancelOnFirstWrite) Write(p []byte) (int, error) {
	if !w.canceled {
		w.canceled = true
		w.cancel()
	}
	return len(p), nil
}
