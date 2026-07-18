package redis

import (
	"context"
	"time"

	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

type shortenerCache struct {
	redis Redis
}

func NewShortenerCache(r Redis) *shortenerCache {
	return &shortenerCache{redis: r}
}

func (c *shortenerCache) HealthCheck(ctx context.Context) error {
	return c.redis.Ping(ctx)
}

func (c *shortenerCache) SaveToken(ctx context.Context, token shortener.Token, url shortener.URL, ttl time.Duration) error {
	return c.redis.Set(ctx, string(token), string(url), ttl)
}

func (c *shortenerCache) GetToken(ctx context.Context, tokenStr string) (shortener.URL, bool, error) {
	v, ok, err := c.redis.Get(ctx, tokenStr)
	url := shortener.URL(v)

	return url, ok, err
}

func (c *shortenerCache) DeleteToken(ctx context.Context, tokenStr string) error {
	return c.redis.Delete(ctx, tokenStr)
}
