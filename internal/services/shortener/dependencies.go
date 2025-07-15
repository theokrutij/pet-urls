package shortener

import (
	"context"
	"time"
)

type repository interface {
	// HealthCheck checks that repository is available.
	// If healthy, returns nil.
	HealthCheck(ctx context.Context) error
	// SaveToken saves url token to persistent repository.
	//
	// If token is not unique, err.NotUnique() = true.
	SaveToken(cxt context.Context, token URLToken) error
	// GetToken fetches url token from persistent repository.
	//
	// If tokenStr doesn't match any token, err.NotFound() = true.
	GetToken(ctx context.Context, tokenStr string) (URLToken, error)
	// DeleteToken deletes token from persistent repository.
	DeleteToken(ctx context.Context, tokenStr string) error
}

type cache interface {
	// HealthCheck checks that cache is available.
	// If healthy, returns nil.
	HealthCheck(ctx context.Context) error
	// SaveToken saves token as cache key with url as value.
	SaveToken(ctx context.Context, token Token, url URL, ttl time.Duration) error
	// GetToken fetches token from cache.
	// If token is not present in cache, ok == false
	GetToken(ctx context.Context, tokenStr string) (url URL, ok bool, err error)
	// DeleteToken deletes token from cache.
	DeleteToken(ctx context.Context, tokenStr string) error
}
