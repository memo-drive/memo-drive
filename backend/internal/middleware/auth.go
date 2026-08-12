// Package middleware provides HTTP middleware for the Fiber web framework,
// including JWT authentication, CORS, and rate limiting.
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/memodrive/backend/internal/config"
)

const authSubjectLocal = "auth.subject"
const authSessionIDLocal = "auth.session_id"

// AdminIdentity is the single-user account identity shared by JWT and WebDAV auth.
const AdminIdentity = "admin"

// SessionCookieName is the browser-only JWT cookie name.
const SessionCookieName = "memodrive_session"

// CSRFHeaderName is required on unsafe requests authenticated by browser cookie.
const CSRFHeaderName = "X-MemoDrive-CSRF"

// Claims extends JWT registered claims with an admin role field.
type Claims struct {
	Role      string `json:"role"`
	SessionID string `json:"sid,omitempty"`
	jwt.RegisteredClaims
}

// AuthSessionValidator checks whether a signed JWT still references an active session.
type AuthSessionValidator interface {
	AuthSessionActive(ctx context.Context, id, subject, credentialFingerprint string, now time.Time) (bool, error)
}

// SessionTokenActive reports whether a signed token references an active session.
func SessionTokenActive(ctx context.Context, cfg config.AuthConfig, sessions AuthSessionValidator, tokenString string) bool {
	if sessions == nil || tokenString == "" {
		return false
	}
	_, err := validateSessionToken(ctx, cfg, sessions, tokenString)
	return err == nil
}

// NewAuthMiddleware returns a Fiber middleware that validates JWT bearer tokens.
// If no admin password is configured, authentication is skipped.
func NewAuthMiddleware(cfg config.AuthConfig, sessions AuthSessionValidator) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if cfg.Password == "" {
			return c.Next()
		}
		tokenString, cookieAuth := tokenFromRequest(c)
		if tokenString == "" {
			return fiber.NewError(fiber.StatusUnauthorized, "missing token")
		}
		claims, err := validateSessionToken(c.Context(), cfg, sessions, tokenString)
		if err != nil {
			return fiber.NewError(fiber.StatusUnauthorized, "invalid token")
		}
		if cookieAuth && unsafeMethod(c.Method()) && c.Get(CSRFHeaderName) != "1" {
			return fiber.NewError(fiber.StatusForbidden, "missing CSRF header")
		}
		c.Locals(authSubjectLocal, claims.Subject)
		c.Locals(authSessionIDLocal, claims.SessionID)
		return c.Next()
	}
}

func validateSessionToken(ctx context.Context, cfg config.AuthConfig, sessions AuthSessionValidator, tokenString string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(_ *jwt.Token) (any, error) {
		return []byte(cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid || claims.Role != AdminIdentity {
		return nil, errors.New("invalid token")
	}
	if claims.Subject == "" {
		claims.Subject = AdminIdentity
	}
	if sessions == nil {
		return claims, nil
	}
	if claims.SessionID == "" {
		return nil, errors.New("invalid session")
	}
	active, err := sessions.AuthSessionActive(ctx, claims.SessionID, claims.Subject, AuthCredentialFingerprint(cfg), time.Now().UTC())
	if err != nil || !active {
		return nil, errors.New("invalid session")
	}
	return claims, nil
}

// AuthCredentialFingerprint changes whenever the password or signing key changes.
func AuthCredentialFingerprint(cfg config.AuthConfig) string {
	mac := hmac.New(sha256.New, []byte(cfg.JWTSecret))
	_, _ = mac.Write([]byte(cfg.Password))
	return hex.EncodeToString(mac.Sum(nil))
}

// AuthSubject returns the identity established by NewAuthMiddleware.
func AuthSubject(c *fiber.Ctx) string {
	subject, _ := c.Locals(authSubjectLocal).(string)
	return subject
}

// AuthSessionID returns the revocable session ID established by NewAuthMiddleware.
func AuthSessionID(c *fiber.Ctx) string {
	sessionID, _ := c.Locals(authSessionIDLocal).(string)
	return sessionID
}

// GenerateToken creates a signed JWT for the admin role and returns its expiration time.
func GenerateToken(cfg config.AuthConfig) (string, time.Time, error) {
	return GenerateSessionToken(cfg, "")
}

// GenerateSessionToken creates a signed admin JWT bound to a revocable session ID.
func GenerateSessionToken(cfg config.AuthConfig, sessionID string) (string, time.Time, error) {
	expiresAt := time.Now().Add(cfg.TokenTTL)
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, Claims{
		Role:      AdminIdentity,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   AdminIdentity,
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

func tokenFromRequest(c *fiber.Ctx) (string, bool) {
	header := c.Get("Authorization")
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")), false
	}
	if cookie := c.Cookies(SessionCookieName); cookie != "" {
		return cookie, true
	}
	return "", false
}

func unsafeMethod(method string) bool {
	switch method {
	case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
		return true
	default:
		return false
	}
}
