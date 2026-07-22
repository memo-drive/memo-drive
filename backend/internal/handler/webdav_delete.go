package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func handleWebDAVDelete(c *fiber.Ctx, webdav *service.WebDAVService, resource *service.WebDAVResource) error {
	virtualPath := ""
	if resource != nil {
		virtualPath = resource.VirtualPath
	}
	writeLog := newWebDAVWriteLog(c, virtualPath)
	defer writeLog.finish()
	if resource != nil && resource.File != nil {
		writeLog.withFile(resource.File.ID, 0)
	}
	if err := webdav.Delete(c.Context(), resource); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeLog.fail(fiber.StatusNotFound, err.Error())
			return c.SendStatus(fiber.StatusNotFound)
		}
		writeLog.fail(fiber.StatusInternalServerError, err.Error())
		return err
	}
	writeLog.complete(fiber.StatusNoContent)
	return c.SendStatus(fiber.StatusNoContent)
}
