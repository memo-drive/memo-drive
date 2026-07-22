package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestBodyLimitRejectsRequestsAboveLimitBeforeHandler(t *testing.T) {
	app := fiber.New(fiber.Config{
		BodyLimit:         4,
		StreamRequestBody: true,
	})
	reached := false
	app.Use("/api", BodyLimit(4))
	app.Post("/api/upload/test", func(c *fiber.Ctx) error {
		reached = true
		return c.SendStatus(fiber.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/upload/test", strings.NewReader("12345"))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("api request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized API body to return 413, got %d", resp.StatusCode)
	}
	if reached {
		t.Fatal("expected oversized API body to be rejected before handler")
	}
}
