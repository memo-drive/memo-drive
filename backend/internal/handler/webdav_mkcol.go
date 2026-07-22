package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func handleWebDAVMkcol(c *fiber.Ctx, webdav *service.WebDAVService, virtualPath string) error {
	writeLog := newWebDAVWriteLog(c, virtualPath)
	defer writeLog.finish()
	folder, err := webdav.CreateFolder(c.Context(), virtualPath)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, service.ErrPathConflict) {
			writeLog.fail(fiber.StatusConflict, err.Error())
			return c.SendStatus(fiber.StatusConflict)
		}
		writeLog.fail(fiber.StatusInternalServerError, err.Error())
		return err
	}
	writeLog.withFile(folder.ID, 0)
	writeLog.complete(fiber.StatusCreated)
	return c.SendStatus(fiber.StatusCreated)
}
