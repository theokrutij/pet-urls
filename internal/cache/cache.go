package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

type Cache struct {
	rdb *redis.Client
}

type Config struct {
	RedisURL string // required

	PoolSize               int // default: 5
	MinIdleConns           int // default: 0
	MaxIdleConns           int // default: 1
	ConnMaxIdleTimeSeconds int // default: 5
}

func New(ctx context.Context, config Config) (*Cache, error) {
	redisOptions, err := redis.ParseURL(config.RedisURL)
	if err != nil {
		return nil, err
	}
	redisOptions = applyAppConfig(redisOptions, config)
	rdb := redis.NewClient(redisOptions)

	go func() {
		<-ctx.Done()
		rdb.Close()
	}()

	return &Cache{rdb: rdb}, nil
}

func applyAppConfig(redisOptions *redis.Options, appConfig Config) *redis.Options {
	if appConfig.PoolSize == 0 {
		redisOptions.PoolSize = 5
	} else {
		redisOptions.PoolSize = appConfig.PoolSize
	}

	redisOptions.MinIdleConns = appConfig.MinIdleConns

	if appConfig.MaxIdleConns == 0 {
		redisOptions.MaxIdleConns = 1
	} else {
		redisOptions.MaxIdleConns = appConfig.MaxIdleConns
	}

	if appConfig.ConnMaxIdleTimeSeconds == 0 {
		redisOptions.ConnMaxIdleTime = 5 * time.Second
	} else {
		redisOptions.ConnMaxIdleTime = time.Duration(appConfig.ConnMaxIdleTimeSeconds) * time.Second
	}

	return redisOptions
}

func (c *Cache) HealthCheck(ctx context.Context) error {
	pong, err := c.rdb.Ping(ctx).Result()
	if err != nil {
		return err
	}
	if pong != "PONG" {
		return errors.New("redis didn't pong")
	}

	return nil
}

func (c *Cache) SaveToken(ctx context.Context, token shortener.Token, url shortener.URL, ttl time.Duration) error {
	return c.set(ctx, string(token), string(url), ttl)
}

func (c *Cache) GetToken(ctx context.Context, tokenStr string) (shortener.URL, bool, error) {
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

func (c *Cache) DeleteToken(ctx context.Context, tokenStr string) error {
	return c.delete(ctx, tokenStr)
}

func (c *Cache) set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, value, ttl).Err()
}

func (c *Cache) get(ctx context.Context, key string) (string, error) {
	val, err := c.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", fmt.Errorf("get from cache, key: %s, err: %w", key, err)
	} else if err != nil {
		return "", fmt.Errorf("get from cache, key: %s, err: %w", key, err)
	}

	return val, nil
}

func (c *Cache) delete(ctx context.Context, key string) error {
	return c.rdb.Del(ctx, key).Err()
}
