// Package handler implements HTTP route handlers for the MemoDrive REST API.
// Each handler wraps one or more service-layer components and registers routes
// on a Fiber router group.
package handler

import "github.com/gofiber/fiber/v2"

// RegisterHealth adds the health check endpoint.
func RegisterHealth(router fiber.Router) {
	router.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"ok":      true,
			"service": "memodrive-backend",
		})
	})
}
