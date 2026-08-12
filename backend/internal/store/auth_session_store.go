package store

import (
	"context"
	"time"

	"github.com/memodrive/backend/internal/model"
)

// CreateAuthSession persists a newly issued login session.
func (s *Store) CreateAuthSession(ctx context.Context, session *model.AuthSession) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO auth_sessions (id, subject, credential_fingerprint, created_at, expires_at, revoked_at)
VALUES (?, ?, ?, ?, ?, NULL)`, session.ID, session.Subject, session.CredentialFingerprint, session.CreatedAt, session.ExpiresAt)
	return err
}

// AuthSessionActive reports whether a session exists and remains usable.
func (s *Store) AuthSessionActive(ctx context.Context, id, subject, credentialFingerprint string, now time.Time) (bool, error) {
	var active bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS(
  SELECT 1 FROM auth_sessions
  WHERE id = ? AND subject = ? AND credential_fingerprint = ?
    AND revoked_at IS NULL AND expires_at > ?
)`, id, subject, credentialFingerprint, now).Scan(&active)
	return active, err
}

// RevokeAuthSession makes one login session unusable immediately.
func (s *Store) RevokeAuthSession(ctx context.Context, id string, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = COALESCE(revoked_at, ?)
WHERE id = ?`, revokedAt, id)
	return err
}

// RevokeAllAuthSessions makes every session for one subject unusable.
func (s *Store) RevokeAllAuthSessions(ctx context.Context, subject string, revokedAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `
UPDATE auth_sessions
SET revoked_at = COALESCE(revoked_at, ?)
WHERE subject = ?`, revokedAt, subject)
	return err
}
