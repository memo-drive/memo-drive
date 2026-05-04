package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

func CORS() fiber.Handler {
	return cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept, Authorization, Upload-Offset, Upload-Chunk-Index",
		AllowMethods: "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	})
}
