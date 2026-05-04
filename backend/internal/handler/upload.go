package handler

import (
	"errors"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

type UploadHandler struct {
	uploads  *service.UploadService
	pipeline *service.PipelineService
}

func NewUploadHandler(uploads *service.UploadService, pipeline *service.PipelineService) *UploadHandler {
	return &UploadHandler{uploads: uploads, pipeline: pipeline}
}

func (h *UploadHandler) Register(router fiber.Router) {
	router.Post("/upload/init", h.init)
	router.Get("/upload/sessions", h.listSessions)
	router.Delete("/upload/sessions", h.clearSessions)
	router.Delete("/upload/sessions/:id", h.deleteSession)
	router.Get("/upload/:id", h.getSession)
	router.Delete("/upload/:id", h.cancel)
	router.Post("/upload/:id/complete", h.complete)
	router.Patch("/upload/:id", h.patch)
}

func (h *UploadHandler) init(c *fiber.Ctx) error {
	var input service.InitUploadInput
	if err := c.BodyParser(&input); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid upload payload")
	}
	session, err := h.uploads.Init(c.Context(), input)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(session)
}

func (h *UploadHandler) patch(c *fiber.Ctx) error {
	session, err := h.uploads.GetSession(c.Context(), c.Params("id"))
	if err != nil {
		return uploadError(err)
	}
	chunkIndex, err := chunkIndexFromRequest(c, session.ChunkSize)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	session, err = h.uploads.SaveChunk(c.Context(), c.Params("id"), chunkIndex, c.Body())
	if err != nil {
		return uploadError(err)
	}
	return c.JSON(fiber.Map{
		"upload_id":       session.ID,
		"uploaded_chunks": session.UploadedChunks,
	})
}

func (h *UploadHandler) complete(c *fiber.Ctx) error {
	file, err := h.uploads.Complete(c.Context(), c.Params("id"))
	if err != nil {
		return uploadError(err)
	}
	task, err := h.pipeline.Enqueue(c.Context(), file)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{
		"file":    file,
		"task_id": task.ID,
	})
}

func (h *UploadHandler) getSession(c *fiber.Ctx) error {
	session, err := h.uploads.GetSession(c.Context(), c.Params("id"))
	if err != nil {
		return uploadError(err)
	}
	return c.JSON(session)
}

func (h *UploadHandler) cancel(c *fiber.Ctx) error {
	if err := h.uploads.CancelSession(c.Context(), c.Params("id")); err != nil {
		return uploadError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *UploadHandler) listSessions(c *fiber.Ctx) error {
	limit := c.QueryInt("limit", 100)
	sessions, err := h.uploads.ListSessions(c.Context(), limit)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"sessions": sessions})
}

func (h *UploadHandler) deleteSession(c *fiber.Ctx) error {
	if err := h.uploads.DeleteSession(c.Context(), c.Params("id")); err != nil {
		return uploadError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *UploadHandler) clearSessions(c *fiber.Ctx) error {
	count, err := h.uploads.ClearSessions(c.Context())
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"deleted": count})
}

func uploadError(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fiber.NewError(fiber.StatusNotFound, "upload session not found")
	}
	return fiber.NewError(fiber.StatusBadRequest, err.Error())
}

func chunkIndexFromRequest(c *fiber.Ctx, chunkSize int64) (int, error) {
	if value := c.Query("chunk"); value != "" {
		return strconv.Atoi(value)
	}
	if value := c.Get("Upload-Chunk-Index"); value != "" {
		return strconv.Atoi(value)
	}
	if value := c.Get("Upload-Offset"); value != "" {
		return service.ChunkIndexFromOffset(value, chunkSize)
	}
	return 0, nil
}
