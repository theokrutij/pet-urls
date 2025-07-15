package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

type cache struct {
	rdb *redis.Client
}

type Config struct {
	Addr string
}

func New(ctx context.Context, config *Config) *cache {
	rdb := redis.NewClient(&redis.Options{Addr: config.Addr})

	go func() {
		<-ctx.Done()
		rdb.Close()
	}()

	return &cache{
		rdb: rdb,
	}
}

func (c *cache) HealthCheck(ctx context.Context) error {
	pong, err := c.rdb.Ping(ctx).Result()
	if err != nil {
		return err
	}
	if pong != "PONG" {
		return errors.New("redis didn't pong")
	}

	return nil
}

func (c *cache) SaveToken(ctx context.Context, token shortener.Token, url shortener.URL, ttl time.Duration) error {
	return c.set(ctx, string(token), string(url), ttl)
}

func (c *cache) GetToken(ctx context.Context, tokenStr string) (shortener.URL, bool, error) {
	v, err := c.get(ctx, tokenStr)
	url := shortener.URL(v)
	if errors.Is(err, redis.Nil) {
		return url, false, nil
	}
	if err != nil {
		return url, false, err
	}

	return url, true, nil
}

func (c *cache) DeleteToken(ctx context.Context, tokenStr string) error {
	return c.delete(ctx, tokenStr)
}

func (c *cache) set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *cache) get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("get from cache, key: %s, err: %w", key, err)
	} else if err != nil {
		return "", fmt.Errorf("get from cache, key: %s, err: %w", key, err)
	}

	return val, nil
}

func (c *cache) delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}
