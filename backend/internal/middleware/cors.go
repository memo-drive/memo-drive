package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/memodrive/backend/internal/config"
)

// CORS returns a Fiber middleware restricted to the configured browser origins.
func CORS(cfg config.CORSConfig) fiber.Handler {
	corsConfig := cors.Config{
		AllowOrigins:     strings.Join(cfg.AllowedOrigins, ","),
		AllowCredentials: cfg.AllowCredentials,
		AllowHeaders:     "Origin, Content-Type, Accept, Authorization, Upload-Offset, Upload-Chunk-Index, " + CSRFHeaderName,
		AllowMethods:     "GET,POST,PUT,PATCH,DELETE,OPTIONS",
	}
	if len(cfg.AllowedOrigins) == 0 {
		corsConfig.AllowOriginsFunc = func(string) bool { return false }
	}
	return cors.New(corsConfig)
}
