package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/memodrive/backend/internal/config"
)

func TestServerStreamsWebDAVBodyBeyondBufferedUploadThreshold(t *testing.T) {
	cfg := &config.Config{
		Storage: config.StorageConfig{
			ChunkSize:   4,
			MaxFileSize: 2 * 1024 * 1024,
		},
	}
	app := fiber.New(httpConfig(cfg))
	reached := false
	app.Put("/dav/large.md", func(c *fiber.Ctx) error {
		reached = true
		if c.Context().Request.BodyStream() == nil {
			t.Fatal("expected large WebDAV request body to be streamed")
		}
		if _, err := io.Copy(io.Discard, c.Context().Request.BodyStream()); err != nil {
			t.Fatalf("read WebDAV stream: %v", err)
		}
		return c.SendStatus(fiber.StatusNoContent)
	})

	body := strings.Repeat("x", int(cfg.Storage.ChunkSize)+1024*1024+1)
	req := httptest.NewRequest(http.MethodPut, "/dav/large.md", strings.NewReader(body))
	resp, err := app.Test(req, 5000)
	if err != nil {
		t.Fatalf("large WebDAV PUT: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent || !reached {
		t.Fatalf("expected large WebDAV request to reach handler, got status %d", resp.StatusCode)
	}
}

func TestServerDoesNotJSONEncodeWebDAVErrors(t *testing.T) {
	app := fiber.New(httpConfig(&config.Config{Storage: config.StorageConfig{ChunkSize: 4}}))
	app.Get("/dav/failure", func(c *fiber.Ctx) error {
		return errors.New("storage failed")
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/dav/failure", nil))
	if err != nil {
		t.Fatalf("WebDAV error request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "application/json") || strings.Contains(string(body), `"error"`) {
		t.Fatalf("expected WebDAV error not to be JSON, got content-type %q body %q", resp.Header.Get("Content-Type"), body)
	}
}

func TestServerAccessLoggerSkipsWebDAVPropfind(t *testing.T) {
	var logs bytes.Buffer
	logConfig := httpLoggerConfig()
	logConfig.Output = &logs
	app := fiber.New(httpConfig(&config.Config{Storage: config.StorageConfig{ChunkSize: 4}}))
	app.Use(logger.New(logConfig))
	app.Add("PROPFIND", "/dav", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusMultiStatus)
	})

	resp, err := app.Test(httptest.NewRequest("PROPFIND", "/dav", nil))
	if err != nil {
		t.Fatalf("PROPFIND /dav: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("expected PROPFIND to return 207, got %d", resp.StatusCode)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected access logger to skip WebDAV PROPFIND, got %q", logs.String())
	}
}
