// Package middleware provides HTTP middleware for the Fiber web framework,
// including JWT authentication, CORS, and rate limiting.
package middleware

import (
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/memodrive/backend/internal/config"
)

// Claims extends JWT registered claims with an admin role field.
type Claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// NewAuthMiddleware returns a Fiber middleware that validates JWT bearer tokens.
// If no admin password is configured, authentication is skipped.
func NewAuthMiddleware(cfg config.AuthConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Password == "" {
			return c.Next()
		}
		tokenString := tokenFromRequest(c)
		if tokenString == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing token")
		}
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (any, error) {
			return []byte(cfg.JWTSecret), nil
		})
		if err != nil || !token.Valid || claims.Role != "admin" {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}
		return c.Next()
	}
}

// GenerateToken creates a signed JWT for the admin role and returns its expiration time.
func GenerateToken(cfg config.AuthConfig) (string, time.Time, error) {
	expiresAt := time.Now().Add(cfg.TokenTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Role: "admin",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	signed, err := token.SignedString([]byte(cfg.JWTSecret))
	return signed, expiresAt, err
}

// ValidatePassword checks the provided password against the configured admin password.
// If no password is configured, all attempts succeed.
func ValidatePassword(cfg config.AuthConfig, password string) error {
	if cfg.Password == "" {
		return nil
	}
	if password != cfg.Password {
		return errors.New("invalid password")
	}
	return nil
}

func tokenFromRequest(c *fiber.Ctx) string {
	header := c.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	return c.Query("token")
}
