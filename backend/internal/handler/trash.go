package handler

import (
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
)

type TrashHandler struct {
	files *service.FileService
}

func NewTrashHandler(files *service.FileService) *TrashHandler {
	return &TrashHandler{files: files}
}

func (h *TrashHandler) Register(router fiber.Router) {
	router.Get("/trash", h.list)
	router.Post("/trash/:id/restore", h.restore)
	router.Delete("/trash/:id", h.purge)
	router.Delete("/trash", h.empty)
}

func (h *TrashHandler) list(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit", "100"))
	files, err := h.files.ListTrashed(c.Context(), limit)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"files": files})
}

func (h *TrashHandler) restore(c *fiber.Ctx) error {
	file, err := h.files.Restore(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	return c.JSON(file)
}

func (h *TrashHandler) purge(c *fiber.Ctx) error {
	if err := h.files.Purge(c.Context(), c.Params("id")); err != nil {
		return mapStoreError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *TrashHandler) empty(c *fiber.Ctx) error {
	purged, err := h.files.EmptyTrash(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"purged": purged})
}
