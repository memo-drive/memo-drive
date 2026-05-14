package middleware

import "github.com/gofiber/fiber/v2"

// RateLimit returns a no-op rate limiting middleware (placeholder for future implementation).
func RateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		return c.Next()
	}
}
