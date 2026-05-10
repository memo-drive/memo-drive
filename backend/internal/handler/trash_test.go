package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func TestTrashRestoreRejectsActiveFile(t *testing.T) {
	app, cleanup := newTrashHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/trash/active/restore", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("restore request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTrashPurgeRejectsActiveFile(t *testing.T) {
	app, cleanup := newTrashHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/trash/active", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("purge request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestTrashPurgeMissingFileReturnsNotFound(t *testing.T) {
	app, cleanup := newTrashHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/trash/missing", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("purge request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestTrashEmptyPurgesTrashEntriesOnly(t *testing.T) {
	app, cleanup := newTrashHandlerTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/trash", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("empty trash request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var emptyBody struct {
		Purged int `json:"purged"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emptyBody); err != nil {
		t.Fatalf("decode empty response: %v", err)
	}
	if emptyBody.Purged != 1 {
		t.Fatalf("expected one purged trash entry, got %d", emptyBody.Purged)
	}

	req = httptest.NewRequest(http.MethodGet, "/trash", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("list trash request failed: %v", err)
	}
	defer resp.Body.Close()
	var listBody struct {
		Files []model.File `json:"files"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listBody); err != nil {
		t.Fatalf("decode trash list response: %v", err)
	}
	if len(listBody.Files) != 0 {
		t.Fatalf("expected empty trash list, got %#v", listBody.Files)
	}

	req = httptest.NewRequest(http.MethodGet, "/files/active", nil)
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("get active file request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected active file to remain available, got %d", resp.StatusCode)
	}
}

func newTrashHandlerTestApp(t *testing.T) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	cfg := &config.Config{
		Storage: config.StorageConfig{
			Root:         storageRoot,
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
	for _, file := range []*model.File{
		{ID: "active", Name: "active.txt", Path: "/", StoragePath: "active.txt", Size: 6, MimeType: "text/plain", Status: model.FileStatusReady},
		{ID: "trashed", Name: "trashed.txt", Path: "/", StoragePath: "trashed.txt", Size: 7, MimeType: "text/plain", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}
	if err := db.SoftDeleteFile(context.Background(), "trashed", "trashed"); err != nil {
		t.Fatalf("soft delete trashed: %v", err)
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	NewTrashHandler(service.NewFileService(cfg, db, nil)).Register(app)
	return app, func() {
		_ = db.Close()
	}
}
