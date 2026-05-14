package handler

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

// FileHandler handles file CRUD, listing, download, search, and thumbnail endpoints.
type FileHandler struct {
	files    *service.FileService
	searches *service.SearchService
}

// NewFileHandler creates a new FileHandler.
func NewFileHandler(files *service.FileService, searches *service.SearchService) *FileHandler {
	return &FileHandler{files: files, searches: searches}
}

func (h *FileHandler) Register(router fiber.Router) {
	router.Get("/files", h.list)
	router.Post("/files/search", h.search)
	router.Get("/files/:id", h.get)
	router.Get("/files/:id/download", h.download)
	router.Head("/files/:id/download", h.download)
	router.Get("/files/:id/thumbnail", h.thumbnail)
	router.Get("/files/:id/metadata", h.metadata)
	router.Put("/files/:id", h.renameMove)
	router.Delete("/files/:id", h.delete)
	router.Post("/folders", h.createFolder)
}

func (h *FileHandler) list(c *fiber.Ctx) error {
	files, err := h.files.List(c.Context(), c.Query("path", "/"), c.Query("sort", "name"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"files": files})
}

func (h *FileHandler) search(c *fiber.Ctx) error {
	if h.searches == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search service is not configured")
	}
	var body service.FileSearchRequest
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid search payload")
	}
	resp, err := h.searches.SearchFiles(c.Context(), body)
	if err != nil {
		return aiError(err)
	}
	return c.JSON(resp)
}

func (h *FileHandler) createFolder(c *fiber.Ctx) error {
	var body struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid folder payload")
	}
	folder, err := h.files.CreateFolder(c.Context(), body.Path, body.Name)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(folder)
}

func (h *FileHandler) get(c *fiber.Ctx) error {
	file, err := h.files.Get(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	return c.JSON(file)
}

func (h *FileHandler) metadata(c *fiber.Ctx) error {
	meta, err := h.files.Metadata(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	c.Set("Content-Type", "application/json")
	return c.SendString(meta.MetaJSON)
}

func (h *FileHandler) thumbnail(c *fiber.Ctx) error {
	path, err := h.files.ThumbnailPath(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	return c.SendFile(path)
}

func (h *FileHandler) renameMove(c *fiber.Ctx) error {
	var body struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid file payload")
	}
	if strings.TrimSpace(body.Name) == "" && strings.TrimSpace(body.Path) == "" {
		return fiber.NewError(fiber.StatusBadRequest, "name or path is required")
	}
	file, err := h.files.RenameMove(c.Context(), c.Params("id"), body.Name, body.Path)
	if err != nil {
		if errors.Is(err, service.ErrPathConflict) {
			return fiber.NewError(fiber.StatusConflict, err.Error())
		}
		return mapStoreError(err)
	}
	return c.JSON(file)
}

func (h *FileHandler) delete(c *fiber.Ctx) error {
	if err := h.files.Delete(c.Context(), c.Params("id")); err != nil {
		return mapStoreError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *FileHandler) download(c *fiber.Ctx) error {
	file, absPath, err := h.files.DownloadPath(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	handle, err := os.Open(absPath)
	if err != nil {
		return err
	}
	stat, err := handle.Stat()
	if err != nil {
		_ = handle.Close()
		return err
	}
	c.Set("Accept-Ranges", "bytes")
	c.Set("Content-Type", file.MimeType)
	c.Set("Content-Disposition", inlineDisposition(file.Name))
	c.Set("X-Content-Type-Options", "nosniff")

	rangeHeader := c.Get("Range")
	if rangeHeader == "" {
		c.Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
		log.Printf("level=info component=file event=download_begin file_id=%s name=%q size=%d range=false", file.ID, file.Name, stat.Size())
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
		log.Printf("level=warn component=file event=download_range_invalid file_id=%s name=%q range=%q size=%d err=%q", file.ID, file.Name, rangeHeader, stat.Size(), err)
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
	log.Printf("level=info component=file event=download_begin file_id=%s name=%q size=%d range=true start=%d end=%d length=%d", file.ID, file.Name, stat.Size(), start, end, length)
	return c.SendStream(section, int(length))
}

type readCloser struct {
	io.Reader
	io.Closer
}

func parseRange(header string, size int64) (int64, int64, error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, errors.New("only bytes ranges are supported")
	}
	spec := strings.TrimSpace(strings.TrimPrefix(header, "bytes="))
	if strings.Contains(spec, ",") {
		return 0, 0, errors.New("multiple ranges are not supported")
	}
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, errors.New("invalid range")
	}
	if size <= 0 {
		return 0, 0, errors.New("empty file")
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if suffix <= 0 {
			return 0, 0, errors.New("invalid suffix range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, err
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, err
		}
	}
	if start < 0 || end < start || end >= size {
		return 0, 0, errors.New("range outside file")
	}
	return start, end, nil
}

func inlineDisposition(name string) string {
	base := filepath.Base(name)
	fallback := asciiFilenameFallback(base)
	return fmt.Sprintf(`inline; filename="%s"; filename*=UTF-8''%s`, quoteHeaderValue(fallback), encodeRFC5987(base))
}

func asciiFilenameFallback(name string) string {
	ext := filepath.Ext(name)
	if !isASCIIHeaderValue(ext) {
		ext = ""
	}
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	var b strings.Builder
	lastUnderscore := false
	for _, r := range base {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-', r == '.', r == ' ':
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_':
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	fallback := strings.Trim(b.String(), " ._")
	if fallback == "" {
		fallback = "download"
	}
	return fallback + ext
}

func quoteHeaderValue(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func isASCIIHeaderValue(value string) bool {
	for _, r := range value {
		if r < 0x20 || r > 0x7e || r == '"' || r == '\\' || r == ';' {
			return false
		}
	}
	return true
}

func encodeRFC5987(value string) string {
	var b strings.Builder
	for _, ch := range []byte(value) {
		if isRFC5987AttrChar(ch) {
			b.WriteByte(ch)
			continue
		}
		_, _ = fmt.Fprintf(&b, "%%%02X", ch)
	}
	return b.String()
}

func isRFC5987AttrChar(ch byte) bool {
	return ch >= 'a' && ch <= 'z' ||
		ch >= 'A' && ch <= 'Z' ||
		ch >= '0' && ch <= '9' ||
		ch == '!' || ch == '#' || ch == '$' || ch == '&' || ch == '+' ||
		ch == '-' || ch == '.' || ch == '^' || ch == '_' || ch == '`' ||
		ch == '|' || ch == '~'
}

func mapStoreError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	if errors.Is(err, service.ErrPathConflict) {
		return fiber.NewError(fiber.StatusConflict, err.Error())
	}
	if errors.Is(err, service.ErrNotInTrash) {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	if errors.Is(err, service.ErrServiceUnavailable) {
		return fiber.NewError(fiber.StatusServiceUnavailable, err.Error())
	}
	return err
}
