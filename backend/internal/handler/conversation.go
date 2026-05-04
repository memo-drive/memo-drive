package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/service"
)

type ConversationHandler struct {
	svc *service.ConversationService
}

func NewConversationHandler(svc *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{svc: svc}
}

func (h *ConversationHandler) Register(router fiber.Router) {
	router.Get("/conversations", h.list)
	router.Get("/conversations/:id", h.get)
	router.Patch("/conversations/:id", h.rename)
	router.Delete("/conversations/:id", h.delete)
}

func (h *ConversationHandler) list(c *fiber.Ctx) error {
	if h.svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "conversation service is not configured")
	}
	limit, _ := strconv.Atoi(c.Query("limit", "50"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))
	items, err := h.svc.List(c.UserContext(), limit, offset)
	if err != nil {
		return mapStoreError(err)
	}
	return c.JSON(fiber.Map{"conversations": items})
}

func (h *ConversationHandler) get(c *fiber.Ctx) error {
	if h.svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "conversation service is not configured")
	}
	conv, messages, err := h.svc.Get(c.UserContext(), c.Params("id"))
	if err != nil {
		return mapStoreError(err)
	}
	return c.JSON(fiber.Map{"conversation": conv, "messages": messages})
}

func (h *ConversationHandler) rename(c *fiber.Ctx) error {
	if h.svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "conversation service is not configured")
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid payload")
	}
	title := strings.TrimSpace(body.Title)
	if title == "" {
		return fiber.NewError(fiber.StatusBadRequest, "title is required")
	}
	if err := h.svc.Rename(c.UserContext(), c.Params("id"), title); err != nil {
		return mapStoreError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *ConversationHandler) delete(c *fiber.Ctx) error {
	if h.svc == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "conversation service is not configured")
	}
	if err := h.svc.Delete(c.UserContext(), c.Params("id")); err != nil {
		return mapStoreError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}
