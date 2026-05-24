package handler

import (
	"encoding/base64"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

const webDAVRealm = `Basic realm="MemoDrive WebDAV"`
const webDAVAllowedMethods = "OPTIONS, PROPFIND, GET, HEAD, PUT, MKCOL, MOVE, COPY, DELETE"
const webDAVVirtualPathLocal = "webdav.virtual_path"
const webDAVResourceLocal = "webdav.resource"
const webDAVAuthFailureLimit = 5

const webDAVAuthFailureWindow = time.Minute

var webDAVCustomRequestMethods = []string{
	"PROPFIND",
	"MKCOL",
	"MOVE",
	"COPY",
	"LOCK",
	"UNLOCK",
	"PROPPATCH",
	"REPORT",
	"SEARCH",
}

// WebDAVRequestMethods returns Fiber request methods with WebDAV verbs included.
func WebDAVRequestMethods(base []string) []string {
	seen := make(map[string]struct{}, len(base)+len(webDAVCustomRequestMethods))
	methods := make([]string, 0, len(base)+len(webDAVCustomRequestMethods))
	for _, method := range base {
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		methods = append(methods, method)
	}
	for _, method := range webDAVCustomRequestMethods {
		if _, ok := seen[method]; ok {
			continue
		}
		seen[method] = struct{}{}
		methods = append(methods, method)
	}
	return methods
}

// IsWebDAVPath reports whether path belongs to the fixed WebDAV endpoint.
func IsWebDAVPath(path string) bool {
	return webDAVPath(path)
}

// RegisterWebDAV adds the optional WebDAV endpoint when it is enabled.
func RegisterWebDAV(router fiber.Router, cfg *config.Config, services ...*service.WebDAVService) {
	if cfg == nil || !cfg.WebDAV.Enabled {
		return
	}
	var webdav *service.WebDAVService
	if len(services) > 0 {
		webdav = services[0]
	}
	authFailures := newWebDAVAuthFailureLimiter()
	handler := func(c *fiber.Ctx) error {
		if !webDAVPath(c.Path()) && !webDAVMissingSlashMountPathAllowed(c.Method(), c.Path()) && !webDAVRootMountPathAllowed(c.Method(), c.Path()) {
			return c.Next()
		}
		if !webDAVAuthorized(c, cfg.Auth) {
			if !authFailures.allow(c.IP()) {
				c.Set("Retry-After", strconv.Itoa(int(webDAVAuthFailureWindow/time.Second)))
				logWebDAVRequestRejected(c, "", fiber.StatusTooManyRequests, "auth_rate_limited", nil)
				return c.SendStatus(fiber.StatusTooManyRequests)
			}
			c.Set("WWW-Authenticate", webDAVRealm)
			logWebDAVRequestRejected(c, "", fiber.StatusUnauthorized, "auth_failed", nil)
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		authFailures.reset(c.IP())
		virtualPath, ok := webDAVVirtualPath(c)
		if !ok {
			logWebDAVRequestRejected(c, "", fiber.StatusBadRequest, "invalid_path", nil)
			return c.SendStatus(fiber.StatusBadRequest)
		}
		c.Locals(webDAVVirtualPathLocal, virtualPath)
		logWebDAVRequestBegin(c, virtualPath)
		if c.Method() == fiber.MethodOptions {
			setWebDAVCapabilityHeaders(c)
			return c.SendStatus(fiber.StatusNoContent)
		}
		if !webDAVMethodAllowed(c.Method()) {
			setWebDAVCapabilityHeaders(c)
			logWebDAVRequestRejected(c, virtualPath, fiber.StatusMethodNotAllowed, "method_not_allowed", nil)
			return c.SendStatus(fiber.StatusMethodNotAllowed)
		}
		if webdav != nil && webDAVMethodRequiresExistingResource(c.Method()) {
			resource, err := webdav.Resolve(c.Context(), virtualPath)
			if errors.Is(err, store.ErrNotFound) {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotFound, "resource_not_found", err)
				return c.SendStatus(fiber.StatusNotFound)
			}
			if errors.Is(err, service.ErrPathConflict) {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusConflict, "path_conflict", err)
				return c.SendStatus(fiber.StatusConflict)
			}
			if err != nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusInternalServerError, "resolve_failed", err)
				return err
			}
			c.Locals(webDAVResourceLocal, resource)
		}
		if webdav != nil && c.Method() == fiber.MethodPut && webDAVPreconditionHeadersPresent(c) {
			if resource, err := webdav.Resolve(c.Context(), virtualPath); err == nil {
				c.Locals(webDAVResourceLocal, resource)
			} else if errors.Is(err, service.ErrPathConflict) {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusConflict, "path_conflict", err)
				return c.SendStatus(fiber.StatusConflict)
			} else if err != nil && !errors.Is(err, store.ErrNotFound) {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusInternalServerError, "resolve_failed", err)
				return err
			}
		}
		if webDAVWriteMethod(c.Method()) {
			resource, _ := c.Locals(webDAVResourceLocal).(*service.WebDAVResource)
			if !checkWebDAVWritePreconditions(c, resource) {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusPreconditionFailed, "precondition_failed", nil)
				return c.SendStatus(fiber.StatusPreconditionFailed)
			}
		}
		if c.Method() == "PROPFIND" {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			resource, _ := c.Locals(webDAVResourceLocal).(*service.WebDAVResource)
			return handleWebDAVPropfind(c, webdav, resource)
		}
		if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			resource, _ := c.Locals(webDAVResourceLocal).(*service.WebDAVResource)
			return handleWebDAVDownload(c, webdav, resource)
		}
		if c.Method() == fiber.MethodPut {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			return handleWebDAVPut(c, webdav, virtualPath)
		}
		if c.Method() == "MKCOL" {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			return handleWebDAVMkcol(c, webdav, virtualPath)
		}
		if c.Method() == fiber.MethodDelete {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			resource, _ := c.Locals(webDAVResourceLocal).(*service.WebDAVResource)
			return handleWebDAVDelete(c, webdav, resource)
		}
		if c.Method() == "MOVE" {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			resource, _ := c.Locals(webDAVResourceLocal).(*service.WebDAVResource)
			return handleWebDAVMove(c, webdav, resource)
		}
		if c.Method() == "COPY" {
			if webdav == nil {
				logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "service_unavailable", nil)
				return c.SendStatus(fiber.StatusNotImplemented)
			}
			resource, _ := c.Locals(webDAVResourceLocal).(*service.WebDAVResource)
			return handleWebDAVCopy(c, webdav, resource)
		}
		logWebDAVRequestRejected(c, virtualPath, fiber.StatusNotImplemented, "not_implemented", nil)
		return c.SendStatus(fiber.StatusNotImplemented)
	}
	router.Use(handler)
}

func webDAVPath(path string) bool {
	return path == "/dav" || strings.HasPrefix(path, "/dav/")
}

func webDAVMissingSlashMountPath(path string) bool {
	return strings.HasPrefix(path, "/dav") && path != "/dav" && !strings.HasPrefix(path, "/dav/")
}

func webDAVMissingSlashMountPathAllowed(method, path string) bool {
	if !webDAVMissingSlashMountPath(path) {
		return false
	}
	return webDAVWritePathCompatMethod(method)
}

func webDAVRootMountPath(path string) bool {
	if path == "" || path == "/" || !strings.HasPrefix(path, "/") {
		return false
	}
	if webDAVPath(path) || webDAVMissingSlashMountPath(path) {
		return false
	}
	trimmed := strings.TrimSuffix(path, "/")
	if trimmed == "" || trimmed == "/" || strings.Contains(strings.TrimPrefix(trimmed, "/"), "/") {
		return false
	}
	return !webDAVReservedRootMountPath(trimmed)
}

func webDAVRootMountPathAllowed(method, path string) bool {
	if !webDAVRootMountPath(path) {
		return false
	}
	return webDAVWritePathCompatMethod(method)
}

func webDAVWritePathCompatMethod(method string) bool {
	switch method {
	case fiber.MethodPut, "MKCOL", "MOVE", "COPY", fiber.MethodDelete:
		return true
	default:
		return false
	}
}

func webDAVReservedRootMountPath(path string) bool {
	segment := strings.TrimPrefix(strings.TrimSuffix(path, "/"), "/")
	if decoded, err := url.PathUnescape(segment); err == nil {
		segment = decoded
	}
	switch strings.ToLower(segment) {
	case "api", "assets", "dav", "files", "favicon.ico", "index.html", "m", "trash":
		return true
	default:
		return false
	}
}

func webDAVVirtualPath(c *fiber.Ctx) (string, bool) {
	rawPath := c.OriginalURL()
	if queryStart := strings.IndexByte(rawPath, '?'); queryStart >= 0 {
		rawPath = rawPath[:queryStart]
	}
	if rawPath == "" {
		rawPath = c.Path()
	}
	return webDAVVirtualPathFromRawPath(rawPath)
}

func webDAVVirtualPathFromRawPath(rawPath string) (string, bool) {
	if rawPath == "/dav" || rawPath == "/dav/" {
		return "/", true
	}
	rawVirtual := ""
	if strings.HasPrefix(rawPath, "/dav/") {
		rawVirtual = strings.TrimPrefix(rawPath, "/dav")
	} else if webDAVMissingSlashMountPath(rawPath) {
		rawVirtual = "/" + strings.TrimPrefix(rawPath, "/dav")
	} else if webDAVRootMountPath(rawPath) {
		rawVirtual = rawPath
	} else {
		return "", false
	}
	lowerRawVirtual := strings.ToLower(rawVirtual)
	if strings.Contains(lowerRawVirtual, "%2f") || strings.Contains(lowerRawVirtual, "%5c") {
		return "", false
	}
	decoded, err := url.PathUnescape(rawVirtual)
	if err != nil || !utf8.ValidString(decoded) {
		return "", false
	}
	if decoded == "" || decoded == "/" {
		return "/", true
	}
	if !strings.HasPrefix(decoded, "/") || strings.Contains(decoded, "\x00") || strings.Contains(decoded, "\\") || strings.Contains(decoded, "//") {
		return "", false
	}
	decoded = strings.TrimSuffix(decoded, "/")
	segments := strings.Split(strings.TrimPrefix(decoded, "/"), "/")
	for i, segment := range segments {
		if !webDAVValidPathSegment(segment) {
			return "", false
		}
		if i == 0 && strings.EqualFold(segment, ".trash") {
			return "", false
		}
	}
	return "/" + strings.Join(segments, "/"), true
}

func webDAVValidPathSegment(segment string) bool {
	if segment == "" || segment == "." || segment == ".." || !utf8.ValidString(segment) {
		return false
	}
	if strings.Contains(segment, "\x00") || strings.Contains(segment, "/") || strings.Contains(segment, "\\") {
		return false
	}
	if strings.TrimSpace(segment) != segment || strings.HasSuffix(segment, ".") {
		return false
	}
	if strings.Trim(segment, ". ") == "" {
		return false
	}
	if strings.HasPrefix(segment, ".") && (len(segment) == 1 || segment[1] == '.' || segment[1] == ' ') {
		return false
	}
	return !strings.ContainsAny(segment, `:*?"<>|`)
}

func setWebDAVCapabilityHeaders(c *fiber.Ctx) {
	c.Set("DAV", "1")
	c.Set("Allow", webDAVAllowedMethods)
}

func webDAVMethodAllowed(method string) bool {
	switch method {
	case fiber.MethodOptions, "PROPFIND", fiber.MethodGet, fiber.MethodHead, fiber.MethodPut, "MKCOL", "MOVE", "COPY", fiber.MethodDelete:
		return true
	default:
		return false
	}
}

func webDAVMethodRequiresExistingResource(method string) bool {
	switch method {
	case "PROPFIND", fiber.MethodGet, fiber.MethodHead, "MOVE", "COPY", fiber.MethodDelete:
		return true
	default:
		return false
	}
}

func webDAVAuthorized(c *fiber.Ctx, cfg config.AuthConfig) bool {
	if cfg.Password == "" {
		return true
	}
	header := c.Get("Authorization")
	if !strings.HasPrefix(header, "Basic ") {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(header, "Basic ")))
	if err != nil {
		return false
	}
	username, password, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return false
	}
	return username == "admin" && password == cfg.Password
}

type webDAVAuthFailureLimiter struct {
	mu       sync.Mutex
	failures map[string]webDAVAuthFailure
	now      func() time.Time
}

type webDAVAuthFailure struct {
	count   int
	resetAt time.Time
}

func newWebDAVAuthFailureLimiter() *webDAVAuthFailureLimiter {
	return &webDAVAuthFailureLimiter{
		failures: map[string]webDAVAuthFailure{},
		now:      time.Now,
	}
}

func (l *webDAVAuthFailureLimiter) allow(client string) bool {
	if strings.TrimSpace(client) == "" {
		client = "unknown"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	failure := l.failures[client]
	if failure.resetAt.IsZero() || !now.Before(failure.resetAt) {
		failure = webDAVAuthFailure{resetAt: now.Add(webDAVAuthFailureWindow)}
	}
	failure.count++
	l.failures[client] = failure
	return failure.count <= webDAVAuthFailureLimit
}

func (l *webDAVAuthFailureLimiter) reset(client string) {
	if strings.TrimSpace(client) == "" {
		client = "unknown"
	}
	l.mu.Lock()
	delete(l.failures, client)
	l.mu.Unlock()
}
