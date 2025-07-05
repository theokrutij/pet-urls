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
	// SaveToken saves token to cache with provided cache key TTL.
	//
	// NOTE: cache key TTL does not correlate with token's inner TTL.
	SaveToken(ctx context.Context, token URLToken, ttl time.Duration) error
	// GetToken fetches token from cache.
	// If token is not present in cache, ok == false
	GetToken(ctx context.Context, tokenStr string) (token URLToken, ok bool, err error)
	// DeleteToken deletes token from cache.
	DeleteToken(ctx context.Context, tokenStr string) error
}
