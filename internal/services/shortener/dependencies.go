package shortener

import (
	"context"
	"time"
)

type repository interface {
	// TODO: docs
	HealthCheck(ctx context.Context) error
	SaveToken(cxt context.Context, token URLToken) error
	GetToken(ctx context.Context, tokenStr string) (URLToken, error)
	DeleteToken(ctx context.Context, tokenStr string) error
}

type cache interface {
	// TODO: docs

	SaveToken(ctx context.Context, token URLToken, ttl time.Duration) error
	GetToken(ctx context.Context, tokenStr string) (URLToken, error)
	DeleteToken(ctx context.Context, tokenStr string) error
}
