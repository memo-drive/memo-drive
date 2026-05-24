package middleware

import (
	"io"

	"github.com/gofiber/fiber/v2"
)

// BodyLimit keeps API routes behind an explicit request body cap when the
// server is configured to stream larger WebDAV request bodies.
func BodyLimit(maxBytes int64) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if maxBytes <= 0 {
			return c.Next()
		}
		contentLength := int64(c.Request().Header.ContentLength())
		if contentLength > maxBytes {
			return fiber.ErrRequestEntityTooLarge
		}
		if contentLength >= 0 {
			return c.Next()
		}
		stream := c.Context().Request.BodyStream()
		if stream == nil {
			if int64(len(c.Body())) > maxBytes {
				return fiber.ErrRequestEntityTooLarge
			}
			return c.Next()
		}
		body, err := io.ReadAll(io.LimitReader(stream, maxBytes+1))
		if err != nil {
			return err
		}
		if int64(len(body)) > maxBytes {
			return fiber.ErrRequestEntityTooLarge
		}
		c.Context().Request.SetBody(body)
		return c.Next()
	}
}
