package middleware

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
)

type rateWindow struct {
	count   int
	resetAt time.Time
}

// RateLimiter enforces independent fixed-window limits for API request classes.
type RateLimiter struct {
	cfg            config.RateLimitConfig
	trustedProxies []*net.IPNet
	mu             sync.Mutex
	windows        map[string]rateWindow
}

// NewRateLimiter creates an in-memory limiter shared by the server routes.
func NewRateLimiter(cfg config.RateLimitConfig, trustedProxyCIDRs []string) *RateLimiter {
	trustedProxies := make([]*net.IPNet, 0, len(trustedProxyCIDRs))
	for _, cidr := range trustedProxyCIDRs {
		_, network, err := net.ParseCIDR(cidr)
		if err == nil {
			trustedProxies = append(trustedProxies, network)
		}
	}
	return &RateLimiter{
		cfg:            cfg,
		trustedProxies: trustedProxies,
		windows:        make(map[string]rateWindow),
	}
}

// API limits authenticated API requests according to their request class.
func (l *RateLimiter) API() fiber.Handler {
	return func(c *fiber.Ctx) error {
		scope := "write"
		limit := l.cfg.WriteRequests
		if strings.HasPrefix(c.Path(), "/api/upload/") {
			scope = "upload"
			limit = l.cfg.UploadRequests
		} else if strings.HasPrefix(c.Path(), "/api/ai/") || c.Path() == "/api/files/search" {
			scope = "ai_search"
			limit = l.cfg.AIRequests
		} else if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			scope = "read"
			limit = l.cfg.ReadRequests
		}
		allowed, retryAfter := l.allow(scope, l.requestIdentity(c), limit)
		if allowed {
			return c.Next()
		}
		return writeRateLimitError(c, scope, retryAfter)
	}
}

func (l *RateLimiter) requestIdentity(c *fiber.Ctx) string {
	if subject := AuthSubject(c); subject != "" {
		return "subject:" + subject
	}
	return "ip:" + l.clientIP(c)
}

func (l *RateLimiter) clientIP(c *fiber.Ctx) string {
	peer := c.Context().RemoteIP()
	if !l.isTrustedProxy(peer) {
		return peer.String()
	}
	forwarded := strings.Split(c.Get(fiber.HeaderXForwardedFor), ",")
	for index := len(forwarded) - 1; index >= 0; index-- {
		candidate := net.ParseIP(strings.TrimSpace(forwarded[index]))
		if candidate == nil {
			return peer.String()
		}
		if !l.isTrustedProxy(candidate) {
			return candidate.String()
		}
	}
	return peer.String()
}

func (l *RateLimiter) isTrustedProxy(ip net.IP) bool {
	for _, network := range l.trustedProxies {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// LoginFailures limits only unsuccessful password checks and resets on success.
func (l *RateLimiter) LoginFailures() fiber.Handler {
	return func(c *fiber.Ctx) error {
		identity := l.clientIP(c)
		if blocked, retryAfter := l.blocked("login", identity, l.cfg.LoginFailures); blocked {
			return writeRateLimitError(c, "login", retryAfter)
		}
		err := c.Next()
		if loginUnauthorized(c, err) {
			allowed, retryAfter := l.allow("login", identity, l.cfg.LoginFailures)
			if !allowed {
				return writeRateLimitError(c, "login", retryAfter)
			}
			return err
		}
		if err == nil && c.Response().StatusCode() < fiber.StatusBadRequest {
			l.reset("login", identity)
		}
		return err
	}
}

func (l *RateLimiter) blocked(scope, identity string, limit int) (bool, int) {
	if limit <= 0 {
		return false, 0
	}
	now := time.Now()
	key := scope + "|" + identity

	l.mu.Lock()
	defer l.mu.Unlock()
	window, exists := l.windows[key]
	if !exists || !now.Before(window.resetAt) {
		delete(l.windows, key)
		return false, 0
	}
	if window.count < limit {
		return false, 0
	}
	return true, retryAfterSeconds(window.resetAt.Sub(now))
}

func loginUnauthorized(c *fiber.Ctx, err error) bool {
	var fiberErr *fiber.Error
	return (errors.As(err, &fiberErr) && fiberErr.Code == fiber.StatusUnauthorized) ||
		(err == nil && c.Response().StatusCode() == fiber.StatusUnauthorized)
}

func writeRateLimitError(c *fiber.Ctx, scope string, retryAfter int) error {
	c.Set(fiber.HeaderRetryAfter, strconv.Itoa(retryAfter))
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"error": fiber.Map{
			"code":      "rate_limited",
			"message":   "too many requests",
			"retryable": true,
			"details": fiber.Map{
				"scope": scope,
			},
		},
	})
}

func (l *RateLimiter) allow(scope, identity string, limit int) (bool, int) {
	if limit <= 0 {
		return true, 0
	}
	now := time.Now()
	key := scope + "|" + identity

	l.mu.Lock()
	defer l.mu.Unlock()
	window := l.windows[key]
	if window.resetAt.IsZero() || !now.Before(window.resetAt) {
		window = rateWindow{resetAt: now.Add(l.cfg.Window)}
	}
	if window.count >= limit {
		l.windows[key] = window
		return false, retryAfterSeconds(window.resetAt.Sub(now))
	}
	window.count++
	l.windows[key] = window
	return true, 0
}

func retryAfterSeconds(remaining time.Duration) int {
	retryAfter := int((remaining + time.Second - 1) / time.Second)
	if retryAfter < 1 {
		return 1
	}
	return retryAfter
}

func (l *RateLimiter) reset(scope, identity string) {
	l.mu.Lock()
	delete(l.windows, scope+"|"+identity)
	l.mu.Unlock()
}
