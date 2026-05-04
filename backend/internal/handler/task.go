package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
)

type TaskHandler struct {
	pipeline *service.PipelineService
}

func NewTaskHandler(pipeline *service.PipelineService) *TaskHandler {
	return &TaskHandler{pipeline: pipeline}
}

func (h *TaskHandler) Register(router fiber.Router) {
	router.Get("/tasks/:id", h.get)
}

func (h *TaskHandler) get(c *fiber.Ctx) error {
	task, err := h.pipeline.GetTask(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	return c.JSON(task)
}
