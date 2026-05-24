package handler

import (
	"errors"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func handleWebDAVMove(c *fiber.Ctx, webdav *service.WebDAVService, resource *service.WebDAVResource) error {
	virtualPath := ""
	if resource != nil {
		virtualPath = resource.VirtualPath
	}
	writeLog := newWebDAVWriteLog(c, virtualPath)
	defer writeLog.finish()
	if resource != nil && resource.File != nil {
		writeLog.withFile(resource.File.ID, 0)
	}
	destination, ok := webDAVDestinationPath(c)
	if !ok {
		writeLog.fail(fiber.StatusBadRequest, "invalid destination")
		return c.SendStatus(fiber.StatusBadRequest)
	}
	writeLog.withDestination(destination)
	result, err := webdav.Move(c.Context(), service.WebDAVMoveInput{
		Source:          resource,
		DestinationPath: destination,
		Overwrite:       webDAVOverwriteAllowed(c),
	})
	if err != nil {
		if errors.Is(err, service.ErrPreconditionFailed) {
			writeLog.fail(fiber.StatusPreconditionFailed, err.Error())
			return c.SendStatus(fiber.StatusPreconditionFailed)
		}
		if errors.Is(err, store.ErrNotFound) {
			writeLog.fail(fiber.StatusConflict, err.Error())
			return c.SendStatus(fiber.StatusConflict)
		}
		if errors.Is(err, service.ErrPathConflict) {
			writeLog.fail(fiber.StatusConflict, err.Error())
			return c.SendStatus(fiber.StatusConflict)
		}
		writeLog.fail(fiber.StatusInternalServerError, err.Error())
		return err
	}
	writeLog.withFile(result.File.ID, 0)
	if result.Overwritten {
		writeLog.complete(fiber.StatusNoContent)
		return c.SendStatus(fiber.StatusNoContent)
	}
	writeLog.complete(fiber.StatusCreated)
	return c.SendStatus(fiber.StatusCreated)
}

func webDAVDestinationPath(c *fiber.Ctx) (string, bool) {
	raw := strings.TrimSpace(c.Get("Destination"))
	if raw == "" {
		return "", false
	}
	destination, err := url.Parse(raw)
	if err != nil || destination.Scheme == "" || destination.Host == "" {
		return "", false
	}
	if !strings.EqualFold(destination.Scheme, c.Protocol()) || !strings.EqualFold(destination.Host, c.Hostname()) {
		return "", false
	}
	return webDAVVirtualPathFromRawPath(destination.EscapedPath())
}

func webDAVOverwriteAllowed(c *fiber.Ctx) bool {
	return !strings.EqualFold(strings.TrimSpace(c.Get("Overwrite")), "F")
}
