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

func TestUploadCancelRejectsMergingSession(t *testing.T) {
	uploads, db, cfg := newUploadServiceTestHarness(t)
	session := &model.UploadSession{
		ID:             "upload-1",
		FileName:       "large.bin",
		FileSize:       10,
		ChunkSize:      5,
		UploadedChunks: []int{0, 1},
		DestPath:       "/",
		Status:         "merging",
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(filepath.Join(cfg.Storage.TempDir, session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	if err := uploads.CancelSession(context.Background(), session.ID); err == nil {
		t.Fatal("expected cancelling a merging session to return an error")
	}

	updated, err := db.GetUploadSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if updated.Status != "merging" {
		t.Fatalf("expected status to remain merging, got %q", updated.Status)
	}
	if _, err := os.Stat(filepath.Join(cfg.Storage.TempDir, session.ID)); err != nil {
		t.Fatalf("expected merging temp dir to remain: %v", err)
	}
}

func TestUploadCompleteFailureAfterMergingMarksSessionFailed(t *testing.T) {
	uploads, db, _ := newUploadServiceTestHarness(t)
	session := &model.UploadSession{
		ID:             "upload-1",
		FileName:       "large.bin",
		FileSize:       10,
		ChunkSize:      5,
		UploadedChunks: []int{0, 1},
		DestPath:       "/",
		Status:         model.UploadStatusUploading,
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(uploads.sessionDir(session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.WriteFile(uploads.chunkPath(session.ID, 0), []byte("12345"), 0o644); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	if _, err := uploads.Complete(context.Background(), session.ID); err == nil {
		t.Fatal("expected complete to fail when a recorded chunk file is missing")
	}

	updated, err := db.GetUploadSession(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get upload session: %v", err)
	}
	if updated.Status != model.UploadStatusFailed {
		t.Fatalf("expected failed status, got %q", updated.Status)
	}
}

func TestUploadCompleteStoresMOVAsVideoQuickTimeWithoutChangingOriginalFile(t *testing.T) {
	uploads, db, cfg := newUploadServiceTestHarness(t)
	original := []byte("original mov bytes")
	session := &model.UploadSession{
		ID:             "upload-mov",
		FileName:       "Meeting.MOV",
		FileSize:       int64(len(original)),
		ChunkSize:      int64(len(original)),
		UploadedChunks: []int{0},
		DestPath:       "/Videos",
		Status:         model.UploadStatusUploading,
		ExpiresAt:      time.Now().UTC().Add(time.Hour),
	}
	if err := os.MkdirAll(uploads.sessionDir(session.ID), 0o755); err != nil {
		t.Fatalf("create session dir: %v", err)
	}
	if err := os.WriteFile(uploads.chunkPath(session.ID, 0), original, 0o644); err != nil {
		t.Fatalf("write upload chunk: %v", err)
	}
	if err := db.CreateUploadSession(context.Background(), session); err != nil {
		t.Fatalf("create upload session: %v", err)
	}

	file, err := uploads.Complete(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("complete mov upload: %v", err)
	}

	if file.MimeType != "video/quicktime" {
		t.Fatalf("expected MOV to be stored as video/quicktime, got %q", file.MimeType)
	}
	if filepath.Ext(file.StoragePath) != ".MOV" {
		t.Fatalf("expected storage path to keep original extension, got %q", file.StoragePath)
	}
	stored, err := os.ReadFile(filepath.Join(cfg.Storage.Root, filepath.FromSlash(file.StoragePath)))
	if err != nil {
		t.Fatalf("read stored original file: %v", err)
	}
	if string(stored) != string(original) {
		t.Fatalf("expected stored file to keep original bytes, got %q", string(stored))
	}
}

func newUploadServiceTestHarness(t *testing.T) (*UploadService, *store.Store, *config.Config) {
	t.Helper()
	root := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         filepath.Join(root, "files"),
			DBPath:       filepath.Join(root, "db", "memodrive.db"),
			TempDir:      filepath.Join(root, "tmp"),
			ThumbnailDir: filepath.Join(root, "thumbs"),
			MaxFileSize:  1024 * 1024,
			ChunkSize:    5,
			UploadTTL:    time.Hour,
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
	files := NewFileService(cfg, db, nil)
	return NewUploadService(cfg, db, files), db, cfg
}
