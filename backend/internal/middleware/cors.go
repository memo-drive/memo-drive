package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// CORS returns a Fiber middleware that allows cross-origin requests from any origin.
// It includes custom headers needed for chunked uploads.
func CORS() fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, Upload-Offset, Upload-Chunk-Index",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	})
}
