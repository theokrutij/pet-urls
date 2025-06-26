package shortener

import (
	"context"
)

type Service interface {
	// HealthCheck runs healthschecks on all dependencies
	HealthCheck(ctx context.Context) error

	// GenerateToken generates random base58-encoded token and
	// sets ttl specified in Config.TokenTTL (default: 1 hour).
	//
	//	- If url cannot be interpreted as valid HTTP url, returns ErrInvalidURL
	GenerateToken(ctx context.Context, url string) (URLToken, error)

	// ResolveToken resolves token to an HTTP URL.
	//
	//	- If token does not match any token in store, returns ErrTokenDoesNotExist.
	//	- If token matches an expired token, returns ErrTokenExpired
	ResolveToken(ctx context.Context, tokenStr string) (url URL, err error)

	// CreateTokenWithOwner creates new token with ownership rights.
	// If token is not provided, generates random base58-encoded token.
	// If ttl is not provided, sets ttl specified in Config.TokenTTL (default: 1 hour)
	//
	//	- If token.URL cannot be interpreted as valid HTTP url, returns ErrInvalidURL.
	//	- If token.ExpiresAt is in the past, returns opaque error.
	//	- If token.OwnerID is nil, returns an opaque error.
	// CreateTokenWithOwner(ctx context.Context, token URLTokenWithOwner) (URLToken, error)

	//
	// DeleteToken(ctx context.Context, tokenStr string, userID []byte) error
}
