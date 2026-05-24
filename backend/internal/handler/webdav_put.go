package handler

import (
	"bytes"
	"errors"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func handleWebDAVPut(c *fiber.Ctx, webdav *service.WebDAVService, virtualPath string) error {
	writeLog := newWebDAVWriteLog(c, virtualPath)
	defer writeLog.finish()
	if c.Get("Content-Range") != "" {
		writeLog.fail(fiber.StatusBadRequest, "content-range is not supported")
		return c.SendStatus(fiber.StatusBadRequest)
	}
	body := webDAVRequestBody(c)
	result, err := webdav.PutFile(c.Context(), service.WebDAVCreateFileInput{
		VirtualPath:   virtualPath,
		Body:          body,
		ContentLength: int64(c.Request().Header.ContentLength()),
		ContentType:   c.Get("Content-Type"),
	})
	if errors.Is(err, service.ErrFileTooLarge) {
		writeLog.fail(fiber.StatusRequestEntityTooLarge, err.Error())
		return c.SendStatus(fiber.StatusRequestEntityTooLarge)
	}
	if errors.Is(err, service.ErrInsufficientStorage) {
		writeLog.fail(fiber.StatusInsufficientStorage, err.Error())
		return c.SendStatus(fiber.StatusInsufficientStorage)
	}
	if errors.Is(err, service.ErrPathConflict) || errors.Is(err, store.ErrNotFound) {
		writeLog.fail(fiber.StatusConflict, err.Error())
		return c.SendStatus(fiber.StatusConflict)
	}
	if err != nil {
		writeLog.fail(fiber.StatusInternalServerError, err.Error())
		return err
	}
	writeLog.withFile(result.File.ID, result.File.Size)
	c.Set("ETag", webDAVETag(result.File))
	if result.Created {
		writeLog.complete(fiber.StatusCreated)
		return c.SendStatus(fiber.StatusCreated)
	}
	writeLog.complete(fiber.StatusNoContent)
	return c.SendStatus(fiber.StatusNoContent)
}

func webDAVRequestBody(c *fiber.Ctx) io.Reader {
	if stream := c.Context().Request.BodyStream(); stream != nil {
		return stream
	}
	return bytes.NewReader(c.Body())
}
