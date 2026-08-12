package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
)

func TestCORSAllowsConfiguredOriginPreflight(t *testing.T) {
	app := fiber.New()
	app.Use(CORS(config.CORSConfig{
		AllowedOrigins: []string{"https://drive.example.com"},
	}))
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	req := httptest.NewRequest(http.MethodOptions, "/api/files", nil)
	req.Header.Set("Origin", "https://drive.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("preflight request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://drive.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want configured origin", got)
	}
}

func TestCORSAddsConfiguredOriginToActualResponse(t *testing.T) {
	app := fiber.New()
	app.Use(CORS(config.CORSConfig{
		AllowedOrigins: []string{"https://drive.example.com"},
	}))
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendString("files") })

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Origin", "https://drive.example.com")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("actual CORS request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("actual request status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "https://drive.example.com" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want configured origin", got)
	}
}

func TestCORSDoesNotAllowCrossOriginWhenAllowlistIsEmpty(t *testing.T) {
	app := fiber.New()
	app.Use(CORS(config.CORSConfig{}))
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendString("files") })

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Origin", "https://untrusted.example.com")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("cross-origin request: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("Access-Control-Allow-Origin = %q, want empty for same-origin-only config", got)
	}
}

func TestCORSAllowsCredentialsOnlyForConfiguredOrigin(t *testing.T) {
	app := fiber.New()
	app.Use(CORS(config.CORSConfig{
		AllowedOrigins:   []string{"https://drive.example.com"},
		AllowCredentials: true,
	}))
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Origin", "https://drive.example.com")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("credentialed CORS request: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Access-Control-Allow-Credentials = %q, want true", got)
	}
}

func TestCORSPreflightAllowsMemoDriveCSRFHeader(t *testing.T) {
	app := fiber.New()
	app.Use(CORS(config.CORSConfig{
		AllowedOrigins:   []string{"https://drive.example.com"},
		AllowCredentials: true,
	}))
	app.Post("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	req := httptest.NewRequest(http.MethodOptions, "/api/files", nil)
	req.Header.Set("Origin", "https://drive.example.com")
	req.Header.Set("Access-Control-Request-Method", http.MethodPost)
	req.Header.Set("Access-Control-Request-Headers", "X-MemoDrive-CSRF")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("CSRF preflight request: %v", err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "x-memodrive-csrf") {
		t.Fatalf("Access-Control-Allow-Headers = %q, want X-MemoDrive-CSRF", got)
	}
}
