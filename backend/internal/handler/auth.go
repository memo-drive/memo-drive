package handler

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/middleware"
)

// AuthHandler handles authentication endpoints: status check and login.
type AuthHandler struct {
	cfg *config.Config
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{cfg: cfg}
}

func (h *AuthHandler) Register(router fiber.Router) {
	router.Get("/auth/status", h.status)
	router.Post("/auth/login", h.login)
}

func (h *AuthHandler) status(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"required": h.cfg.Auth.Password != "",
	})
}

func (h *AuthHandler) login(c *fiber.Ctx) error {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid login payload")
	}
	if err := middleware.ValidatePassword(h.cfg.Auth, body.Password); err != nil {
		log.Printf("level=warn component=auth event=login_failed ip=%s err=%q", c.IP(), err)
		return fiber.NewError(fiber.StatusUnauthorized, err.Error())
	}
	token, expiresAt, err := middleware.GenerateToken(h.cfg.Auth)
	if err != nil {
		log.Printf("level=error component=auth event=token_generate_failed ip=%s err=%q", c.IP(), err)
		return err
	}
	log.Printf("level=info component=auth event=login_success ip=%s expires_at=%s", c.IP(), expiresAt.Format("2006-01-02T15:04:05Z07:00"))
	return c.JSON(fiber.Map{
		"token":      token,
		"expires_at": expiresAt,
	})
}
