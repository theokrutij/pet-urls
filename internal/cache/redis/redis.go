package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type Redis interface {
	Ping(ctx context.Context) error
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, bool, error)
	Delete(ctx context.Context, key string) error
	FlushDB(ctx context.Context) error
}

type redisClient struct {
	rdb *redis.Client
}

type Config struct {
	RedisURL string // required

	PoolSize               int            // default: 5
	MinIdleConns           int            // default: 0
	MaxIdleConns           int            // default: 1
	ConnMaxIdleTimeSeconds int            // default: 5
	Logger                 zerolog.Logger // default: no logging
}

func New(ctx context.Context, config Config) (Redis, error) {
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

	return &redisClient{rdb: rdb}, nil
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

func (r *redisClient) Ping(ctx context.Context) error {
	pong, err := r.rdb.Ping(ctx).Result()
	if err != nil {
		return err
	}
	if pong != "PONG" {
		return errors.New("redis didn't pong")
	}

	return nil
}

func (r *redisClient) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return r.rdb.Set(ctx, key, value, ttl).Err()
}

func (r *redisClient) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := r.rdb.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", false, nil
	} else if err != nil {
		return "", false, fmt.Errorf("get from cache, key: %s, err: %w", key, err)
	}

	return val, true, nil
}

func (r *redisClient) Delete(ctx context.Context, key string) error {
	return r.rdb.Del(ctx, key).Err()
}

func (r *redisClient) FlushDB(ctx context.Context) error {
	return r.rdb.FlushDB(ctx).Err()
}
