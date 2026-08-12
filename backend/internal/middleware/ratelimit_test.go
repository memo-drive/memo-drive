package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/memodrive/backend/internal/config"
)

func TestRateLimiterLimitsReadAPIRequests(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:       time.Minute,
		ReadRequests: 1,
	}, nil)
	app := fiber.New()
	app.Use(limiter.API())
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	first, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/files", nil))
	if err != nil {
		t.Fatalf("first read request: %v", err)
	}
	_ = first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first read status = %d, want %d", first.StatusCode, http.StatusOK)
	}

	second, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/files", nil))
	if err != nil {
		t.Fatalf("second read request: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second read status = %d, want %d", second.StatusCode, http.StatusTooManyRequests)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Fatal("rate-limited response must include Retry-After")
	}
}

func TestRateLimiterReturnsStructuredRetryableError(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:       time.Minute,
		ReadRequests: 1,
	}, nil)
	app := fiber.New()
	app.Use(limiter.API())
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	first, _ := app.Test(httptest.NewRequest(http.MethodGet, "/api/files", nil))
	_ = first.Body.Close()
	second, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/files", nil))
	if err != nil {
		t.Fatalf("rate-limited request: %v", err)
	}
	defer second.Body.Close()

	var body struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if err := json.NewDecoder(second.Body).Decode(&body); err != nil {
		t.Fatalf("decode rate-limit response: %v", err)
	}
	if body.Error.Code != "rate_limited" || !body.Error.Retryable {
		t.Fatalf("rate-limit error = %+v, want retryable rate_limited", body.Error)
	}
}

func TestRateLimiterLimitsWriteAPIRequests(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:        time.Minute,
		WriteRequests: 1,
	}, nil)
	app := fiber.New()
	app.Use(limiter.API())
	app.Post("/api/folders", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusCreated) })

	first, _ := app.Test(httptest.NewRequest(http.MethodPost, "/api/folders", nil))
	_ = first.Body.Close()
	second, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/folders", nil))
	if err != nil {
		t.Fatalf("second write request: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second write status = %d, want %d", second.StatusCode, http.StatusTooManyRequests)
	}
}

func TestRateLimiterUsesDedicatedUploadBucket(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:         time.Minute,
		WriteRequests:  10,
		UploadRequests: 1,
	}, nil)
	app := fiber.New()
	app.Use(limiter.API())
	app.Post("/api/upload/init", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusCreated) })
	app.Patch("/api/upload/:id", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	first, _ := app.Test(httptest.NewRequest(http.MethodPost, "/api/upload/init", nil))
	_ = first.Body.Close()
	second, err := app.Test(httptest.NewRequest(http.MethodPatch, "/api/upload/session-1", nil))
	if err != nil {
		t.Fatalf("upload chunk request: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("upload chunk status = %d, want %d after init consumed upload bucket", second.StatusCode, http.StatusTooManyRequests)
	}
}

func TestRateLimiterUsesDedicatedAIAndSearchBucket(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:        time.Minute,
		WriteRequests: 10,
		AIRequests:    1,
	}, nil)
	app := fiber.New()
	app.Use(limiter.API())
	app.Post("/api/ai/chat", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
	app.Post("/api/files/search", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	first, _ := app.Test(httptest.NewRequest(http.MethodPost, "/api/ai/chat", nil))
	_ = first.Body.Close()
	second, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/files/search", nil))
	if err != nil {
		t.Fatalf("search request: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("search status = %d, want %d after chat consumed AI/search bucket", second.StatusCode, http.StatusTooManyRequests)
	}
}

func TestRateLimiterLimitsRepeatedLoginFailures(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:        time.Minute,
		LoginFailures: 1,
	}, nil)
	app := fiber.New()
	app.Post("/api/auth/login", limiter.LoginFailures(), func(c *fiber.Ctx) error {
		return fiber.NewError(http.StatusUnauthorized, "invalid password")
	})

	first, _ := app.Test(httptest.NewRequest(http.MethodPost, "/api/auth/login", nil))
	_ = first.Body.Close()
	if first.StatusCode != http.StatusUnauthorized {
		t.Fatalf("first login failure status = %d, want %d", first.StatusCode, http.StatusUnauthorized)
	}
	second, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/auth/login", nil))
	if err != nil {
		t.Fatalf("second login failure: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second login failure status = %d, want %d", second.StatusCode, http.StatusTooManyRequests)
	}
	if second.Header.Get("Retry-After") == "" {
		t.Fatal("rate-limited login failure must include Retry-After")
	}
}

func TestRateLimiterBlocksCorrectPasswordAfterFailureLimit(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:        time.Minute,
		LoginFailures: 1,
	}, nil)
	app := fiber.New()
	app.Post("/api/auth/login", limiter.LoginFailures(), func(c *fiber.Ctx) error {
		if c.Get("X-Test-Password") != "correct" {
			return fiber.NewError(http.StatusUnauthorized, "invalid password")
		}
		return c.SendStatus(http.StatusOK)
	})

	wrong, _ := app.Test(httptest.NewRequest(http.MethodPost, "/api/auth/login", nil))
	_ = wrong.Body.Close()
	valid := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	valid.Header.Set("X-Test-Password", "correct")
	blocked, err := app.Test(valid)
	if err != nil {
		t.Fatalf("blocked correct login: %v", err)
	}
	defer blocked.Body.Close()
	if blocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("correct login after failure limit status = %d, want %d", blocked.StatusCode, http.StatusTooManyRequests)
	}
}

func TestRateLimiterResetsLoginFailuresAfterSuccess(t *testing.T) {
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:        time.Minute,
		LoginFailures: 2,
	}, nil)
	app := fiber.New()
	app.Post("/api/auth/login", limiter.LoginFailures(), func(c *fiber.Ctx) error {
		if c.Get("X-Test-Password") != "correct" {
			return fiber.NewError(http.StatusUnauthorized, "invalid password")
		}
		return c.SendStatus(http.StatusOK)
	})

	wrong := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	first, _ := app.Test(wrong)
	_ = first.Body.Close()
	valid := httptest.NewRequest(http.MethodPost, "/api/auth/login", nil)
	valid.Header.Set("X-Test-Password", "correct")
	success, _ := app.Test(valid)
	_ = success.Body.Close()
	afterReset, err := app.Test(httptest.NewRequest(http.MethodPost, "/api/auth/login", nil))
	if err != nil {
		t.Fatalf("login failure after success: %v", err)
	}
	defer afterReset.Body.Close()
	if afterReset.StatusCode != http.StatusUnauthorized {
		t.Fatalf("login failure after success status = %d, want reset counter and %d", afterReset.StatusCode, http.StatusUnauthorized)
	}
}

func TestRateLimiterUsesAuthenticatedSubjectBeforeClientIP(t *testing.T) {
	auth := config.AuthConfig{Password: "password", JWTSecret: "test-secret", TokenTTL: time.Hour}
	tokenForSubject := func(subject string) string {
		t.Helper()
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
			Role: "admin",
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   subject,
				ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			},
		})
		signed, err := token.SignedString([]byte(auth.JWTSecret))
		if err != nil {
			t.Fatalf("sign token for %s: %v", subject, err)
		}
		return signed
	}
	limiter := NewRateLimiter(config.RateLimitConfig{
		Window:       time.Minute,
		ReadRequests: 1,
	}, nil)
	app := fiber.New()
	app.Use(NewAuthMiddleware(auth, nil), limiter.API())
	app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })

	firstReq := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	firstReq.Header.Set("Authorization", "Bearer "+tokenForSubject("session-a"))
	first, _ := app.Test(firstReq)
	_ = first.Body.Close()

	secondReq := httptest.NewRequest(http.MethodGet, "/api/files", nil)
	secondReq.Header.Set("Authorization", "Bearer "+tokenForSubject("session-b"))
	second, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second authenticated request: %v", err)
	}
	defer second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("different subject on same IP status = %d, want independent bucket and %d", second.StatusCode, http.StatusOK)
	}
}

func TestRateLimiterTrustsForwardedForOnlyFromConfiguredProxy(t *testing.T) {
	request := func(origin string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/files", nil)
		req.Header.Set("X-Forwarded-For", origin)
		return req
	}
	newApp := func(trustedProxies []string) *fiber.App {
		limiter := NewRateLimiter(config.RateLimitConfig{
			Window:       time.Minute,
			ReadRequests: 1,
		}, trustedProxies)
		app := fiber.New()
		app.Use(limiter.API())
		app.Get("/api/files", func(c *fiber.Ctx) error { return c.SendStatus(http.StatusOK) })
		return app
	}

	untrustedApp := newApp(nil)
	first, _ := untrustedApp.Test(request("203.0.113.10"))
	_ = first.Body.Close()
	second, _ := untrustedApp.Test(request("203.0.113.11"))
	_ = second.Body.Close()
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("untrusted forwarded address status = %d, want shared peer bucket and %d", second.StatusCode, http.StatusTooManyRequests)
	}

	trustedApp := newApp([]string{"0.0.0.0/32", "::/128"})
	first, _ = trustedApp.Test(request("203.0.113.10"))
	_ = first.Body.Close()
	second, _ = trustedApp.Test(request("203.0.113.11"))
	_ = second.Body.Close()
	if second.StatusCode != http.StatusOK {
		t.Fatalf("trusted forwarded address status = %d, want separate client bucket and %d", second.StatusCode, http.StatusOK)
	}
}
