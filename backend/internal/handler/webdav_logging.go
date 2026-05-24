package handler

import (
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
)

type webDAVWriteLog struct {
	started         time.Time
	method          string
	virtualPath     string
	destinationPath string
	fileID          string
	bytes           int64
	status          int
	err             string
}

func newWebDAVWriteLog(c *fiber.Ctx, virtualPath string) *webDAVWriteLog {
	return &webDAVWriteLog{
		started:     time.Now(),
		method:      c.Method(),
		virtualPath: cleanWebDAVLogPath(virtualPath),
		status:      fiber.StatusInternalServerError,
	}
}

func (l *webDAVWriteLog) withDestination(destinationPath string) {
	l.destinationPath = cleanWebDAVLogPath(destinationPath)
}

func (l *webDAVWriteLog) withFile(fileID string, bytes int64) {
	l.fileID = fileID
	l.bytes = bytes
}

func (l *webDAVWriteLog) complete(status int) {
	l.status = status
	l.err = ""
}

func (l *webDAVWriteLog) fail(status int, err string) {
	l.status = status
	l.err = err
}

func (l *webDAVWriteLog) finish() {
	log.Printf("level=info component=webdav event=write_complete method=%s virtual_path=%q destination_path=%q file_id=%s bytes=%d status=%d duration_ms=%d err=%q",
		l.method, l.virtualPath, l.destinationPath, l.fileID, l.bytes, l.status, time.Since(l.started).Milliseconds(), l.err)
}

func logWebDAVRequestBegin(c *fiber.Ctx, virtualPath string) {
	if !webDAVShouldLogRequestBegin(c.Method()) {
		return
	}
	log.Printf("level=info component=webdav event=request_begin method=%s virtual_path=%q path=%q path_compat=%q protocol=%q forwarded_proto=%q host=%q depth=%q content_length=%d content_type=%q has_content_range=%t has_destination=%t destination=%q overwrite=%q has_if=%t has_if_match=%t has_if_none_match=%t user_agent=%q",
		c.Method(),
		cleanWebDAVLogPath(virtualPath),
		c.Path(),
		webDAVPathCompatReason(c.Path()),
		c.Protocol(),
		c.Get("X-Forwarded-Proto"),
		c.Hostname(),
		strings.TrimSpace(c.Get("Depth")),
		c.Request().Header.ContentLength(),
		c.Get("Content-Type"),
		strings.TrimSpace(c.Get("Content-Range")) != "",
		strings.TrimSpace(c.Get("Destination")) != "",
		webDAVDestinationLogValue(c),
		strings.TrimSpace(c.Get("Overwrite")),
		strings.TrimSpace(c.Get("If")) != "",
		strings.TrimSpace(c.Get("If-Match")) != "",
		strings.TrimSpace(c.Get("If-None-Match")) != "",
		c.Get("User-Agent"),
	)
}

func logWebDAVRequestRejected(c *fiber.Ctx, virtualPath string, status int, reason string, err error) {
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	log.Printf("level=warn component=webdav event=request_rejected method=%s virtual_path=%q path=%q path_compat=%q protocol=%q forwarded_proto=%q host=%q status=%d reason=%q has_destination=%t destination=%q has_if=%t has_if_match=%t has_if_none_match=%t err=%q",
		c.Method(),
		cleanWebDAVLogPath(virtualPath),
		c.Path(),
		webDAVPathCompatReason(c.Path()),
		c.Protocol(),
		c.Get("X-Forwarded-Proto"),
		c.Hostname(),
		status,
		reason,
		strings.TrimSpace(c.Get("Destination")) != "",
		webDAVDestinationLogValue(c),
		strings.TrimSpace(c.Get("If")) != "",
		strings.TrimSpace(c.Get("If-Match")) != "",
		strings.TrimSpace(c.Get("If-None-Match")) != "",
		errText,
	)
}

func webDAVPathCompatReason(path string) string {
	if webDAVMissingSlashMountPath(path) {
		return "missing_slash_after_mount"
	}
	if webDAVRootMountPath(path) {
		return "missing_mount_prefix"
	}
	return ""
}

func webDAVShouldLogRequestBegin(method string) bool {
	switch method {
	case fiber.MethodPut, "MKCOL", "MOVE", "COPY", fiber.MethodDelete, "LOCK", "UNLOCK", "PROPPATCH", "REPORT", "SEARCH":
		return true
	default:
		return false
	}
}

func webDAVDestinationLogValue(c *fiber.Ctx) string {
	raw := strings.TrimSpace(c.Get("Destination"))
	if raw == "" {
		return ""
	}
	destination, err := url.Parse(raw)
	if err != nil {
		return "invalid"
	}
	if destination.Scheme == "" || destination.Host == "" {
		return "relative_or_missing_host"
	}
	return destination.Scheme + "://" + destination.Host + destination.EscapedPath()
}

func cleanWebDAVLogPath(path string) string {
	if path == "" {
		return ""
	}
	return path
}

func webDAVVirtualPathLocalValue(c *fiber.Ctx) string {
	if c == nil {
		return ""
	}
	if virtualPath, ok := c.Locals(webDAVVirtualPathLocal).(string); ok {
		return virtualPath
	}
	return ""
}
