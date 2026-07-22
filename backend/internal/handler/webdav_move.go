package handler

import (
	"errors"
	"net"
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
	destination, reason, ok := webDAVDestinationPathWithReason(c)
	if !ok {
		writeLog.fail(fiber.StatusBadRequest, "invalid destination: "+reason)
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
	destination, _, ok := webDAVDestinationPathWithReason(c)
	return destination, ok
}

func webDAVDestinationPathWithReason(c *fiber.Ctx) (string, string, bool) {
	raw := strings.TrimSpace(c.Get("Destination"))
	if raw == "" {
		return "", "missing", false
	}
	destination, err := url.Parse(raw)
	if err != nil || destination.Scheme == "" || destination.Host == "" {
		return "", "invalid_url", false
	}
	if !webDAVDestinationSchemeAllowed(c, destination.Scheme, destination.Host) {
		return "", "scheme_mismatch", false
	}
	if !strings.EqualFold(destination.Host, c.Hostname()) {
		return "", "host_mismatch", false
	}
	virtualPath, ok := webDAVVirtualPathFromRawPath(destination.EscapedPath())
	if !ok {
		return "", "invalid_path", false
	}
	return virtualPath, "", true
}

func webDAVOverwriteAllowed(c *fiber.Ctx) bool {
	return !strings.EqualFold(strings.TrimSpace(c.Get("Overwrite")), "F")
}

func webDAVDestinationSchemeAllowed(c *fiber.Ctx, destinationScheme, destinationHost string) bool {
	if strings.EqualFold(destinationScheme, c.Protocol()) {
		return true
	}
	if strings.EqualFold(destinationScheme, webDAVFirstHeaderValue(c.Get("X-Forwarded-Proto"))) {
		return true
	}
	if !strings.EqualFold(destinationScheme, "https") || !strings.EqualFold(c.Protocol(), "http") {
		return false
	}
	if webDAVHostPort(destinationHost) == "443" && webDAVHostPort(c.Hostname()) == "443" {
		return true
	}
	return false
}

func webDAVFirstHeaderValue(value string) string {
	if before, _, ok := strings.Cut(value, ","); ok {
		return strings.TrimSpace(before)
	}
	return strings.TrimSpace(value)
}

func webDAVHostPort(host string) string {
	_, port, err := net.SplitHostPort(strings.TrimSpace(host))
	if err != nil {
		return ""
	}
	return port
}
