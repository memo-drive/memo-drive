package handler

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/model"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

func handleWebDAVDownload(c *fiber.Ctx, webdav *service.WebDAVService, resource *service.WebDAVResource) error {
	if resource == nil {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if resource.IsDir() {
		setWebDAVCapabilityHeaders(c)
		return c.SendStatus(fiber.StatusMethodNotAllowed)
	}
	file, absPath, err := webdav.DownloadPath(resource)
	if errors.Is(err, store.ErrNotFound) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if err != nil {
		return err
	}
	return sendWebDAVFile(c, file, absPath)
}

func sendWebDAVFile(c *fiber.Ctx, file *model.File, absPath string) error {
	handle, err := os.Open(absPath)
	if err != nil {
		return err
	}
	stat, err := handle.Stat()
	if err != nil {
		_ = handle.Close()
		return err
	}
	contentType := file.MimeType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.Set("Accept-Ranges", "bytes")
	c.Set("Content-Type", contentType)
	c.Set("Content-Disposition", inlineDisposition(file.Name))
	c.Set("X-Content-Type-Options", "nosniff")
	c.Set("ETag", webDAVETag(file))
	c.Set("Last-Modified", file.UpdatedAt.UTC().Format(http.TimeFormat))

	rangeHeader := c.Get("Range")
	if rangeHeader == "" {
		c.Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
		log.Printf("level=info component=webdav event=download_begin file_id=%s name=%q size=%d range=false", file.ID, file.Name, stat.Size())
		if c.Method() == fiber.MethodHead {
			_ = handle.Close()
			return nil
		}
		return c.SendStream(handle, int(stat.Size()))
	}

	start, end, err := parseRange(rangeHeader, stat.Size())
	if err != nil {
		_ = handle.Close()
		c.Set("Content-Range", fmt.Sprintf("bytes */%d", stat.Size()))
		log.Printf("level=warn component=webdav event=download_range_invalid file_id=%s name=%q range=%q size=%d err=%q", file.ID, file.Name, rangeHeader, stat.Size(), err)
		return fiber.NewError(fiber.StatusRequestedRangeNotSatisfiable, err.Error())
	}
	length := end - start + 1
	c.Status(fiber.StatusPartialContent)
	c.Set("Content-Length", strconv.FormatInt(length, 10))
	c.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, stat.Size()))
	if c.Method() == fiber.MethodHead {
		_ = handle.Close()
		return nil
	}
	section := &readCloser{Reader: io.NewSectionReader(handle, start, length), Closer: handle}
	log.Printf("level=info component=webdav event=download_begin file_id=%s name=%q size=%d range=true start=%d end=%d length=%d", file.ID, file.Name, stat.Size(), start, end, length)
	return c.SendStream(section, int(length))
}
