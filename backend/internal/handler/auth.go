package handler

import (
	"context"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/memodrive/backend/internal/config"
	"github.com/memodrive/backend/internal/middleware"
	"github.com/memodrive/backend/internal/model"
)

type authSessionStore interface {
	CreateAuthSession(ctx context.Context, session *model.AuthSession) error
	AuthSessionActive(ctx context.Context, id, subject, credentialFingerprint string, now time.Time) (bool, error)
	RevokeAuthSession(ctx context.Context, id string, revokedAt time.Time) error
	RevokeAllAuthSessions(ctx context.Context, subject string, revokedAt time.Time) error
}

// AuthHandler handles authentication endpoints: status check and login.
type AuthHandler struct {
	cfg      *config.Config
	sessions authSessionStore
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(cfg *config.Config, sessions authSessionStore) *AuthHandler {
	return &AuthHandler{cfg: cfg, sessions: sessions}
}

func (h *AuthHandler) Register(router fiber.Router) {
	router.Get("/auth/status", h.status)
	router.Post("/auth/login", h.login)
}

// RegisterProtected adds authentication endpoints that require an active session.
func (h *AuthHandler) RegisterProtected(router fiber.Router) {
	router.Post("/auth/logout", h.logout)
}

func (h *AuthHandler) status(c *fiber.Ctx) error {
	required := h.cfg.Auth.Password != ""
	authenticated := !required
	if required && h.sessions != nil {
		authenticated = middleware.SessionTokenActive(
			c.Context(),
			h.cfg.Auth,
			h.sessions,
			c.Cookies(middleware.SessionCookieName),
		)
	}
	return c.JSON(fiber.Map{
		"required":      required,
		"authenticated": authenticated,
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
	sessionID := uuid.NewString()
	token, expiresAt, err := middleware.GenerateSessionToken(h.cfg.Auth, sessionID)
	if err != nil {
		log.Printf("level=error component=auth event=token_generate_failed ip=%s err=%q", c.IP(), err)
		return err
	}
	if h.sessions != nil {
		if err := h.sessions.CreateAuthSession(c.Context(), &model.AuthSession{
			ID:                    sessionID,
			Subject:               middleware.AdminIdentity,
			CredentialFingerprint: middleware.AuthCredentialFingerprint(h.cfg.Auth),
			CreatedAt:             time.Now().UTC(),
			ExpiresAt:             expiresAt.UTC(),
		}); err != nil {
			log.Printf("level=error component=auth event=session_create_failed ip=%s err=%q", c.IP(), err)
			return err
		}
	}
	log.Printf("level=info component=auth event=login_success ip=%s expires_at=%s", c.IP(), expiresAt.Format("2006-01-02T15:04:05Z07:00"))
	c.Cookie(&fiber.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    token,
		Path:     "/api",
		Expires:  expiresAt,
		MaxAge:   int(time.Until(expiresAt) / time.Second),
		HTTPOnly: true,
		Secure:   h.cfg.AppEnv == config.AppEnvProduction,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
	return c.JSON(fiber.Map{
		"token":      token,
		"expires_at": expiresAt,
	})
}

func (h *AuthHandler) logout(c *fiber.Ctx) error {
	sessionID := middleware.AuthSessionID(c)
	if sessionID == "" {
		return fiber.NewError(fiber.StatusUnauthorized, "invalid session")
	}
	if h.sessions == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "session store is unavailable")
	}
	var input struct {
		Scope string `json:"scope"`
	}
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&input); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid logout payload")
		}
	}
	if input.Scope == "" {
		input.Scope = "current"
	}
	now := time.Now().UTC()
	var err error
	switch input.Scope {
	case "current":
		err = h.sessions.RevokeAuthSession(c.Context(), sessionID, now)
	case "all":
		err = h.sessions.RevokeAllAuthSessions(c.Context(), middleware.AuthSubject(c), now)
	default:
		return fiber.NewError(fiber.StatusBadRequest, "logout scope must be current or all")
	}
	if err != nil {
		return err
	}
	c.Cookie(&fiber.Cookie{
		Name:     middleware.SessionCookieName,
		Value:    "",
		Path:     "/api",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HTTPOnly: true,
		Secure:   h.cfg.AppEnv == config.AppEnvProduction,
		SameSite: fiber.CookieSameSiteLaxMode,
	})
	return c.SendStatus(fiber.StatusNoContent)
}
