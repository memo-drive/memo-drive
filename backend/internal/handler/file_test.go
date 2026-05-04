package handler

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func TestParseRangeSupportsPDFJSRanges(t *testing.T) {
	tests := []struct {
		name   string
		header string
		size   int64
		start  int64
		end    int64
	}{
		{name: "open ended", header: "bytes=100-", size: 1000, start: 100, end: 999},
		{name: "bounded", header: "bytes=100-199", size: 1000, start: 100, end: 199},
		{name: "suffix", header: "bytes=-128", size: 1000, start: 872, end: 999},
		{name: "suffix larger than file", header: "bytes=-2000", size: 1000, start: 0, end: 999},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end, err := parseRange(tt.header, tt.size)
			if err != nil {
				t.Fatalf("parseRange returned error: %v", err)
			}
			if start != tt.start || end != tt.end {
				t.Fatalf("expected %d-%d, got %d-%d", tt.start, tt.end, start, end)
			}
		})
	}
}

func TestParseRangeRejectsInvalidRanges(t *testing.T) {
	for _, header := range []string{
		"bytes=100-50",
		"bytes=100-2000",
		"bytes=-0",
		"bytes=0-10,20-30",
		"items=0-10",
	} {
		if _, _, err := parseRange(header, 1000); err == nil {
			t.Fatalf("expected %q to be rejected", header)
		}
	}
}

func TestInlineDispositionUsesASCIIFallbackForUnicodeNames(t *testing.T) {
	header := inlineDisposition("美悦界一品牌管理 （上海）有限公司 _发票金额283.99元.pdf")
	if !strings.HasPrefix(header, "inline;") {
		t.Fatalf("expected inline disposition, got %q", header)
	}
	if !strings.Contains(header, `filename*=`) {
		t.Fatalf("expected RFC 5987 filename*, got %q", header)
	}
	for _, r := range header {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("header contains non-ASCII rune %q in %q", r, header)
		}
	}
}

func TestFileDownloadHeadersAndSuffixRange(t *testing.T) {
	app, cleanup := newFileDownloadTestApp(t, []byte("%PDF-1.7\nhello world\n%%EOF"))
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/files/file-1/download", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("download request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("expected application/pdf content type, got %q", got)
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "inline;") {
		t.Fatalf("expected inline disposition, got %q", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/files/file-1/download", nil)
	req.Header.Set("Range", "bytes=-5")
	resp, err = app.Test(req)
	if err != nil {
		t.Fatalf("range request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("expected 206, got %d", resp.StatusCode)
	}
	if got := resp.Header.Get("Content-Range"); got != "bytes 21-25/26" {
		t.Fatalf("unexpected content range %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read range body: %v", err)
	}
	if string(body) != "%%EOF" {
		t.Fatalf("unexpected range body %q", body)
	}
}

func TestRenameMoveRejectsEmptyPayload(t *testing.T) {
	app, cleanup := newFileRenameTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/files/file-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("rename request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRenameMoveMapsConflictTo409(t *testing.T) {
	app, cleanup := newFileRenameTestApp(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPut, "/files/file-1", strings.NewReader(`{"name":"existing.pdf"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("rename request failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func newFileDownloadTestApp(t *testing.T, content []byte) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	if err := os.WriteFile(filepath.Join(storageRoot, "sample.pdf"), content, 0o644); err != nil {
		t.Fatalf("write sample pdf: %v", err)
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
	file := &model.File{
		ID:          "file-1",
		Name:        "sample.pdf",
		Path:        "/",
		StoragePath: "sample.pdf",
		Size:        int64(len(content)),
		MimeType:    "application/pdf",
		Status:      model.FileStatusReady,
	}
	if err := db.CreateFile(context.Background(), file); err != nil {
		t.Fatalf("create file: %v", err)
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, func() {
		_ = db.Close()
	}
}

func newFileRenameTestApp(t *testing.T) (*fiber.App, func()) {
	t.Helper()
	root := t.TempDir()
	storageRoot := filepath.Join(root, "files")
	if err := os.MkdirAll(storageRoot, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	for _, name := range []string{"sample.pdf", "existing.pdf"} {
		if err := os.WriteFile(filepath.Join(storageRoot, name), []byte("pdf"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
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
		{ID: "file-1", Name: "sample.pdf", Path: "/", StoragePath: "sample.pdf", Size: 3, MimeType: "application/pdf", Status: model.FileStatusReady},
		{ID: "file-2", Name: "existing.pdf", Path: "/", StoragePath: "existing.pdf", Size: 3, MimeType: "application/pdf", Status: model.FileStatusReady},
	} {
		if err := db.CreateFile(context.Background(), file); err != nil {
			t.Fatalf("create file %s: %v", file.ID, err)
		}
	}

	app := fiber.New()
	NewFileHandler(service.NewFileService(cfg, db, nil), nil).Register(app)
	return app, func() {
		_ = db.Close()
	}
}
