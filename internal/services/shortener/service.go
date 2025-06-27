package shortener

import (
	"context"
)

type Service interface {
	// HealthCheck runs healthschecks on all dependencies
	HealthCheck(ctx context.Context) error

	// GenerateToken generates random base58-encoded token and
	// sets default ttl from app-wide config
	//
	//	- If url cannot be interpreted as valid HTTP url, returns ErrInvalidURL
	GenerateToken(ctx context.Context, url string) (URLToken, error)

	// ResolveToken resolves token to an HTTP URL.
	//
	//	- If token does not match any token in store, returns ErrTokenDoesNotExist.
	//	- If token matches an expired token, returns ErrTokenExpired
	ResolveToken(ctx context.Context, tokenStr string) (url URL, err error)

	// CreateTokenWithOwner creates new token with ownership rights.
	// If input.Token is empty, generates random base58-encoded token.
	// If input.TTL <= 0, sets default ttl from app-wide config
	//
	//	- If input.URL cannot be interpreted as valid HTTP url, returns ErrInvalidURL.
	// 	- If input.Token contains more than 64 characters, returns ErrInvalidToken
	//	- If input.Token is not unique, returns ErrTokenIsNotUnique
	CreateTokenWithOwner(ctx context.Context, input CreateTokenInput) (URLToken, error)

	//
	// DeleteToken(ctx context.Context, tokenStr string, userID []byte) error
}
