package model

import "time"

// AuthSession is a revocable login session referenced by a JWT sid claim.
type AuthSession struct {
	ID                    string
	Subject               string
	CredentialFingerprint string
	CreatedAt             time.Time
	ExpiresAt             time.Time
	RevokedAt             *time.Time
}
