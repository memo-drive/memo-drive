package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/middleware"
	"github.com/memodrive/backend/internal/store"
)

func TestSessionCookieAuthenticatesProtectedAPI(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, db).Register(api)
	protected := api.Group("", middleware.NewAuthMiddleware(cfg.Auth, db))
	protected.Get("/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var sessionCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login response did not set session cookie")
	}

	protectedReq := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	protectedReq.AddCookie(sessionCookie)
	protectedResp, err := app.Test(protectedReq)
	if err != nil {
		t.Fatalf("protected request: %v", err)
	}
	defer protectedResp.Body.Close()
	if protectedResp.StatusCode != http.StatusOK {
		t.Fatalf("protected request status = %d, want %d", protectedResp.StatusCode, http.StatusOK)
	}
}

func TestProtectedAPIRejectsSessionTokenInQuery(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, db).Register(api)
	protected := api.Group("", middleware.NewAuthMiddleware(cfg.Auth, db))
	protected.Get("/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var token string
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("login response did not contain session token")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/protected?token="+url.QueryEscape(token), nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("query token request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("query token status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestLogoutCurrentSessionImmediatelyRevokesCookie(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	auth := NewAuthHandler(cfg, db)
	auth.Register(api)
	protected := api.Group("", middleware.NewAuthMiddleware(cfg.Auth, db))
	auth.RegisterProtected(protected)
	protected.Get("/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var sessionCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("login response did not set session cookie")
	}

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	logoutReq.AddCookie(sessionCookie)
	logoutReq.Header.Set("X-MemoDrive-CSRF", "1")
	logoutResp, err := app.Test(logoutReq)
	if err != nil {
		t.Fatalf("logout request: %v", err)
	}
	_ = logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d, want %d", logoutResp.StatusCode, http.StatusNoContent)
	}

	afterReq := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	afterReq.AddCookie(sessionCookie)
	afterResp, err := app.Test(afterReq)
	if err != nil {
		t.Fatalf("request after logout: %v", err)
	}
	defer afterResp.Body.Close()
	if afterResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("request after logout status = %d, want %d", afterResp.StatusCode, http.StatusUnauthorized)
	}
}

func TestLogoutAllRevokesEverySession(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	auth := NewAuthHandler(cfg, db)
	auth.Register(api)
	protected := api.Group("", middleware.NewAuthMiddleware(cfg.Auth, db))
	auth.RegisterProtected(protected)
	protected.Get("/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	login := func() *http.Cookie {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("login request: %v", err)
		}
		_ = resp.Body.Close()
		for _, cookie := range resp.Cookies() {
			if cookie.Name == "memodrive_session" {
				return cookie
			}
		}
		t.Fatal("login response did not set session cookie")
		return nil
	}
	firstCookie := login()
	secondCookie := login()

	logoutReq := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{"scope":"all"}`))
	logoutReq.Header.Set("Content-Type", "application/json")
	logoutReq.Header.Set("X-MemoDrive-CSRF", "1")
	logoutReq.AddCookie(firstCookie)
	logoutResp, err := app.Test(logoutReq)
	if err != nil {
		t.Fatalf("logout all request: %v", err)
	}
	_ = logoutResp.Body.Close()
	if logoutResp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout all status = %d, want %d", logoutResp.StatusCode, http.StatusNoContent)
	}

	for index, cookie := range []*http.Cookie{firstCookie, secondCookie} {
		req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
		req.AddCookie(cookie)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("protected request for session %d: %v", index+1, err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("session %d after logout all status = %d, want %d", index+1, resp.StatusCode, http.StatusUnauthorized)
		}
	}
}

func TestPasswordRotationInvalidatesExistingSessions(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	loginApp := fiber.New()
	loginAPI := loginApp.Group("/api")
	NewAuthHandler(cfg, db).Register(loginAPI)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := loginApp.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var oldCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			oldCookie = cookie
		}
	}
	if oldCookie == nil {
		t.Fatal("login response did not set session cookie")
	}

	rotatedAuth := cfg.Auth
	rotatedAuth.Password = "new-password"
	protectedApp := fiber.New()
	protectedApp.Use(middleware.NewAuthMiddleware(rotatedAuth, db))
	protectedApp.Get("/api/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(oldCookie)
	resp, err := protectedApp.Test(req)
	if err != nil {
		t.Fatalf("request after password rotation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("request after password rotation status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSigningKeyRotationInvalidatesExistingSessions(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	loginApp := fiber.New()
	loginAPI := loginApp.Group("/api")
	NewAuthHandler(cfg, db).Register(loginAPI)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := loginApp.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var oldCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == middleware.SessionCookieName {
			oldCookie = cookie
			break
		}
	}
	if oldCookie == nil {
		t.Fatal("login response did not set session cookie")
	}

	rotatedAuth := cfg.Auth
	rotatedAuth.JWTSecret = "rotated-test-secret"
	protectedApp := fiber.New()
	protectedApp.Use(middleware.NewAuthMiddleware(rotatedAuth, db))
	protectedApp.Get("/api/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.AddCookie(oldCookie)
	resp, err := protectedApp.Test(req)
	if err != nil {
		t.Fatalf("request after signing-key rotation: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("request after signing-key rotation status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestSessionStoreRejectsLegacyJWTWithoutSessionID(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	legacyToken, _, err := middleware.GenerateToken(cfg.Auth)
	if err != nil {
		t.Fatalf("generate legacy token: %v", err)
	}

	app := fiber.New()
	app.Use(middleware.NewAuthMiddleware(cfg.Auth, db))
	app.Get("/api/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer "+legacyToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("legacy token request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("legacy token status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
}

func TestCookieAuthenticatedWriteRequiresCSRFHeader(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, db).Register(api)
	protected := api.Group("", middleware.NewAuthMiddleware(cfg.Auth, db))
	protected.Post("/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var sessionCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("login response did not set session cookie")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/protected", nil)
	req.AddCookie(sessionCookie)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("cookie write request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cookie write without CSRF header status = %d, want %d", resp.StatusCode, http.StatusForbidden)
	}
}

func TestAuthStatusReportsActiveCookieSession(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, db).Register(api)
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var sessionCookie *http.Cookie
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			sessionCookie = cookie
		}
	}
	if sessionCookie == nil {
		t.Fatal("login response did not set session cookie")
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/auth/status", nil)
	statusReq.AddCookie(sessionCookie)
	statusResp, err := app.Test(statusReq)
	if err != nil {
		t.Fatalf("auth status request: %v", err)
	}
	defer statusResp.Body.Close()
	var status struct {
		Required      bool `json:"required"`
		Authenticated bool `json:"authenticated"`
	}
	if err := json.NewDecoder(statusResp.Body).Decode(&status); err != nil {
		t.Fatalf("decode auth status: %v", err)
	}
	if !status.Required || !status.Authenticated {
		t.Fatalf("auth status = %+v, want required and authenticated", status)
	}
}

func TestBearerAuthenticatedWriteDoesNotRequireCSRFHeader(t *testing.T) {
	cfg := authTestConfig(t)
	db, err := store.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, db).Register(api)
	protected := api.Group("", middleware.NewAuthMiddleware(cfg.Auth, db))
	protected.Post("/protected", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusNoContent) })

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginResp, err := app.Test(loginReq)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	_ = loginResp.Body.Close()
	var token string
	for _, cookie := range loginResp.Cookies() {
		if cookie.Name == "memodrive_session" {
			token = cookie.Value
		}
	}
	if token == "" {
		t.Fatal("login response did not set session token")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("bearer write request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("bearer write without CSRF header status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func authTestConfig(t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		AppEnv: config.AppEnvDevelopment,
		Storage: config.StorageConfig{
			DBPath: t.TempDir() + "/memodrive.db",
		},
		Auth: config.AuthConfig{
			Password:  "correct-password",
			JWTSecret: "test-secret",
			TokenTTL:  time.Hour,
		},
	}
}

func TestLoginSetsHTTPOnlySessionCookie(t *testing.T) {
	cfg := &config.Config{
		AppEnv: config.AppEnvDevelopment,
		Auth: config.AuthConfig{
			Password:  "correct-password",
			JWTSecret: "test-secret",
			TokenTTL:  time.Hour,
		},
	}
	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, nil).Register(api)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var sessionCookie *http.Cookie
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "memodrive_session" {
			sessionCookie = cookie
			break
		}
	}
	if sessionCookie == nil {
		t.Fatal("login response must set memodrive_session cookie")
	}
	if !sessionCookie.HttpOnly || sessionCookie.Path != "/api" || sessionCookie.SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie attributes = HttpOnly:%t Path:%q SameSite:%v", sessionCookie.HttpOnly, sessionCookie.Path, sessionCookie.SameSite)
	}

	claims := &middleware.Claims{}
	token, err := jwt.ParseWithClaims(sessionCookie.Value, claims, func(*jwt.Token) (any, error) {
		return []byte(cfg.Auth.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		t.Fatalf("parse session cookie token: valid=%t err=%v", token != nil && token.Valid, err)
	}
	if claims.SessionID == "" {
		t.Fatal("session cookie token must contain a non-empty sid")
	}
}

func TestLoginSetsSecureSessionCookieInProduction(t *testing.T) {
	cfg := &config.Config{
		AppEnv: config.AppEnvProduction,
		Auth: config.AuthConfig{
			Password:  "correct-password",
			JWTSecret: "test-secret",
			TokenTTL:  time.Hour,
		},
	}
	app := fiber.New()
	api := app.Group("/api")
	NewAuthHandler(cfg, nil).Register(api)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct-password"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer resp.Body.Close()
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "memodrive_session" {
			if !cookie.Secure {
				t.Fatal("production session cookie must be Secure")
			}
			return
		}
	}
	t.Fatal("login response did not set session cookie")
}

func TestLoginEndpointRateLimitsRepeatedPasswordFailures(t *testing.T) {
	cfg := &config.Config{Auth: config.AuthConfig{
		Password:  "correct-password",
		JWTSecret: "test-secret",
		TokenTTL:  time.Hour,
	}}
	limiter := middleware.NewRateLimiter(config.RateLimitConfig{
		Window:        time.Minute,
		LoginFailures: 1,
	}, nil)
	app := fiber.New()
	api := app.Group("/api")
	api.Use("/auth/login", limiter.LoginFailures())
	NewAuthHandler(cfg, nil).Register(api)

	login := func() *http.Response {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("login request: %v", err)
		}
		return resp
	}

	first := login()
	_ = first.Body.Close()
	if first.StatusCode != http.StatusUnauthorized {
		t.Fatalf("first login status = %d, want %d", first.StatusCode, http.StatusUnauthorized)
	}
	second := login()
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second login status = %d, want %d", second.StatusCode, http.StatusTooManyRequests)
	}
}
