package cache

import (
	"context"
	"encoding/json"
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

type tokenAsCacheValue struct {
	URL string     `json:"url"`
	Exp *time.Time `json:"exp,omitempty"`
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

func (c *cache) SaveToken(ctx context.Context, token shortener.URLToken, ttl time.Duration) error {
	tcv := tokenAsCacheValue{
		URL: string(token.URL),
		Exp: token.ExpiresAt,
	}

	b, err := json.Marshal(tcv)
	if err != nil {
		return err
	}

	return c.set(ctx, string(token.Token), string(b), ttl)
}

func (c *cache) GetToken(ctx context.Context, tokenStr string) (shortener.URLToken, bool, error) {
	var token shortener.URLToken
	cacheValue, err := c.get(ctx, tokenStr)
	if errors.Is(err, redis.Nil) {
		return token, false, nil
	}
	if err != nil {
		return token, false, err
	}

	var tcv tokenAsCacheValue
	err = json.Unmarshal([]byte(cacheValue), &tcv)
	if err != nil {
		return token, true, err
	}

	token.URL = shortener.URL(tcv.URL)
	token.ExpiresAt = tcv.Exp
	return token, true, nil
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
