package handler

import "github.com/gofiber/fiber/v2"

func RegisterHealth(router fiber.Router) {
	router.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"ok":      true,
			"service": "memodrive-backend",
		})
	})
}
