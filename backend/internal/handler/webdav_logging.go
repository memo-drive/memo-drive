package handler

import (
	"log"
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

func cleanWebDAVLogPath(path string) string {
	if path == "" {
		return ""
	}
	return path
}
