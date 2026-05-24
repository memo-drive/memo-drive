package handler

import (
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
)

func checkWebDAVWritePreconditions(c *fiber.Ctx, resource *service.WebDAVResource) bool {
	ifMatch := strings.TrimSpace(c.Get("If-Match"))
	if ifMatch != "" {
		if resource == nil || resource.File == nil {
			return false
		}
		if ifMatch != "*" && !webDAVETagListContains(ifMatch, webDAVETag(resource.File)) {
			return false
		}
	}
	ifNoneMatch := strings.TrimSpace(c.Get("If-None-Match"))
	if ifNoneMatch == "" {
		return checkWebDAVIfHeader(c, resource)
	}
	if resource == nil || resource.File == nil {
		return checkWebDAVIfHeader(c, resource)
	}
	if ifNoneMatch == "*" {
		return false
	}
	if webDAVETagListContains(ifNoneMatch, webDAVETag(resource.File)) {
		return false
	}
	return checkWebDAVIfHeader(c, resource)
}

func checkWebDAVIfHeader(c *fiber.Ctx, resource *service.WebDAVResource) bool {
	header := strings.TrimSpace(c.Get("If"))
	if header == "" {
		return true
	}
	etag, ok := parseSimpleWebDAVIfETag(header)
	if !ok || resource == nil || resource.File == nil {
		log.Printf("level=warn component=webdav event=if_header_rejected method=%s path=%q reason=%q", c.Method(), c.Path(), "unsupported_or_missing_resource")
		return false
	}
	return etag == webDAVETag(resource.File)
}

func parseSimpleWebDAVIfETag(header string) (string, bool) {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, `([`) || !strings.HasSuffix(header, `])`) {
		return "", false
	}
	value := strings.TrimPrefix(strings.TrimSuffix(header, `])`), `([`)
	if strings.TrimSpace(value) != value || value == "" {
		return "", false
	}
	if !strings.HasPrefix(value, `"`) || !strings.HasSuffix(value, `"`) {
		return "", false
	}
	if strings.ContainsAny(strings.Trim(value, `"`), "[]()<>") {
		return "", false
	}
	return value, true
}

func webDAVETagListContains(header, current string) bool {
	for _, candidate := range strings.Split(header, ",") {
		if strings.TrimSpace(candidate) == current {
			return true
		}
	}
	return false
}

func webDAVWriteMethod(method string) bool {
	switch method {
	case fiber.MethodPut, fiber.MethodDelete, "MOVE", "COPY":
		return true
	default:
		return false
	}
}

func webDAVPreconditionHeadersPresent(c *fiber.Ctx) bool {
	return strings.TrimSpace(c.Get("If-Match")) != "" ||
		strings.TrimSpace(c.Get("If-None-Match")) != "" ||
		strings.TrimSpace(c.Get("If")) != ""
}
