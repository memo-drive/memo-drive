package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
)

// StorageHandler serves storage usage information.
type StorageHandler struct {
	files *service.FileService
}

// NewStorageHandler creates a new StorageHandler.
func NewStorageHandler(files *service.FileService) *StorageHandler {
	return &StorageHandler{files: files}
}

func (h *StorageHandler) Register(router fiber.Router) {
	router.Get("/storage/usage", h.usage)
}

func (h *StorageHandler) usage(c *fiber.Ctx) error {
	usage, err := h.files.StorageUsage(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(usage)
}
