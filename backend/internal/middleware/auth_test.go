package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/memodrive/backend/internal/config"
)

func TestGenerateTokenIdentifiesAdminSubject(t *testing.T) {
	cfg := config.AuthConfig{JWTSecret: "test-secret", TokenTTL: time.Hour}
	tokenString, _, err := GenerateToken(cfg)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(*jwt.Token) (any, error) {
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("parse generated token: valid=%t err=%v", token != nil && token.Valid, err)
	}
	if claims.Subject != "admin" {
		t.Fatalf("generated token subject = %q, want admin", claims.Subject)
	}
}

func TestAuthMiddlewareExposesAuthenticatedSubject(t *testing.T) {
	cfg := config.AuthConfig{Password: "password", JWTSecret: "test-secret", TokenTTL: time.Hour}
	token, _, err := GenerateToken(cfg)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	app := fiber.New()
	app.Use(NewAuthMiddleware(cfg, nil))
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendString(AuthSubject(c)) })

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("authenticated request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if got := string(body); got != "admin" {
		t.Fatalf("authenticated subject = %q, want admin", got)
	}
}

func TestAuthMiddlewareKeepsTokensWithoutSubjectCompatible(t *testing.T) {
	cfg := config.AuthConfig{Password: "password", JWTSecret: "test-secret", TokenTTL: time.Hour}
	legacy := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	token, err := legacy.SignedString([]byte(cfg.JWTSecret))
	if err != nil {
		t.Fatalf("sign legacy token: %v", err)
	}
	app := fiber.New()
	app.Use(NewAuthMiddleware(cfg, nil))
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendString(AuthSubject(c)) })

	req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("legacy token request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || string(body) != "admin" {
		t.Fatalf("legacy token response = %d %q, want 200 admin", resp.StatusCode, body)
	}
}
