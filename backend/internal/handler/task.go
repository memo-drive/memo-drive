package handler

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
	"github.com/memodrive/backend/internal/store"
)

// TaskHandler serves pipeline task status queries.
type TaskHandler struct {
	pipeline *service.PipelineService
}

// NewTaskHandler creates a new TaskHandler.
func NewTaskHandler(pipeline *service.PipelineService) *TaskHandler {
	return &TaskHandler{pipeline: pipeline}
}

func (h *TaskHandler) Register(router fiber.Router) {
	router.Get("/tasks", h.list)
	router.Post("/tasks/:id/retry", h.retry)
	router.Get("/tasks/:id", h.get)
}

func (h *TaskHandler) retry(c *fiber.Ctx) error {
	task, err := h.pipeline.RetryTask(c.Context(), c.Params("id"))
	if err != nil {
		return writeTaskRetryError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"task": task})
}

func writeTaskRetryError(c *fiber.Ctx, err error) error {
	if errors.Is(err, service.ErrPipelineQueueFull) {
		return taskErrorResponse(c, fiber.StatusServiceUnavailable, "pipeline_queue_full", "pipeline queue is full; retry later", true, nil)
	}
	if errors.Is(err, store.ErrNotFound) {
		return taskErrorResponse(c, fiber.StatusNotFound, "task_not_found", "pipeline Task not found", false, fiber.Map{"task_id": c.Params("id")})
	}
	var state *service.TaskNotFailedError
	if errors.As(err, &state) {
		return taskErrorResponse(c, fiber.StatusConflict, "task_not_failed", state.Error(), false, fiber.Map{"task_id": state.TaskID, "status": state.Status})
	}
	var active *service.TaskAlreadyActiveError
	if errors.As(err, &active) {
		return taskErrorResponse(c, fiber.StatusConflict, "task_already_active", active.Error(), false, fiber.Map{"file_id": active.FileID})
	}
	var trashed *service.TaskFileInTrashError
	if errors.As(err, &trashed) {
		return taskErrorResponse(c, fiber.StatusConflict, "file_in_trash", trashed.Error(), false, fiber.Map{"file_id": trashed.FileID})
	}
	log.Printf("level=warn component=task event=retry_failed task_id=%s err=%q", c.Params("id"), err)
	return taskErrorResponse(c, fiber.StatusInternalServerError, "task_retry_failed", "pipeline Task could not be retried", true, nil)
}

func taskErrorResponse(c *fiber.Ctx, status int, code, message string, retryable bool, details fiber.Map) error {
	payload := fiber.Map{
		"code":      code,
		"message":   message,
		"retryable": retryable,
	}
	if details != nil {
		payload["details"] = details
	}
	return c.Status(status).JSON(fiber.Map{"error": payload})
}

func (h *TaskHandler) list(c *fiber.Ctx) error {
	page, err := h.pipeline.ListTasks(
		c.Context(),
		c.Query("status"),
		c.Query("file_id"),
		c.Query("cursor"),
		c.QueryInt("limit", 0),
	)
	if err != nil {
		if errors.Is(err, service.ErrInvalidTaskStatus) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fiber.Map{
					"code":      "invalid_task_status",
					"message":   "status must be pending, processing, done, or failed",
					"retryable": false,
				},
			})
		}
		if errors.Is(err, store.ErrInvalidCursor) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": fiber.Map{
					"code":      "invalid_cursor",
					"message":   "invalid cursor",
					"retryable": false,
				},
			})
		}
		return mapStoreError(err)
	}
	return c.JSON(page)
}

func (h *TaskHandler) get(c *fiber.Ctx) error {
	task, err := h.pipeline.GetTask(c.Context(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	return c.JSON(task)
}
