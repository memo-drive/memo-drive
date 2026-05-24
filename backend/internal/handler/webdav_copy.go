package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func handleWebDAVCopy(c *fiber.Ctx, webdav *service.WebDAVService, resource *service.WebDAVResource) error {
	virtualPath := ""
	if resource != nil {
		virtualPath = resource.VirtualPath
	}
	writeLog := newWebDAVWriteLog(c, virtualPath)
	defer writeLog.finish()
	if resource != nil && resource.File != nil {
		writeLog.withFile(resource.File.ID, resource.File.Size)
	}
	destination, ok := webDAVDestinationPath(c)
	if !ok {
		writeLog.fail(fiber.StatusBadRequest, "invalid destination")
		return c.SendStatus(fiber.StatusBadRequest)
	}
	writeLog.withDestination(destination)
	result, err := webdav.Copy(c.Context(), service.WebDAVCopyInput{
		Source:          resource,
		DestinationPath: destination,
		Overwrite:       webDAVOverwriteAllowed(c),
	})
	if err != nil {
		if errors.Is(err, service.ErrUnsupportedResource) {
			writeLog.fail(fiber.StatusNotImplemented, err.Error())
			return c.SendStatus(fiber.StatusNotImplemented)
		}
		if errors.Is(err, service.ErrPreconditionFailed) {
			writeLog.fail(fiber.StatusPreconditionFailed, err.Error())
			return c.SendStatus(fiber.StatusPreconditionFailed)
		}
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, service.ErrPathConflict) {
			writeLog.fail(fiber.StatusConflict, err.Error())
			return c.SendStatus(fiber.StatusConflict)
		}
		writeLog.fail(fiber.StatusInternalServerError, err.Error())
		return err
	}
	writeLog.withFile(result.File.ID, result.File.Size)
	if result.Created {
		writeLog.complete(fiber.StatusCreated)
		return c.SendStatus(fiber.StatusCreated)
	}
	writeLog.complete(fiber.StatusNoContent)
	return c.SendStatus(fiber.StatusNoContent)
}
