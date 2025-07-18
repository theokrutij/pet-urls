package integration_tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theokrutij/pet-urls/internal/cache"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
)

const (
	testTimeout             = time.Second
	acceptableTimePrecision = time.Second

	testJWTSecret = "abcd"

	EnvKeyTestRedisURL    = "TEST_REDIS_URL"
	EnvKeyTestPostgresDSN = "TEST_POSTGRES_DSN"
)

// testPostgres implements the postgres.Postgres interface.
// It uses pgx.Tx instead of a connection pool,
// and adds a Ping method to satisfy the interface.
type testPostgres struct {
	pgx.Tx
}

func (m testPostgres) Ping(ctx context.Context) error { return nil }

func setupTestPostgres(ctx context.Context, t *testing.T) postgres.Postgres {
	pool, err := pgxpool.New(ctx, os.Getenv(EnvKeyTestPostgresDSN))
	require.NoError(t, err)
	t.Cleanup(func() { pool.Close() })

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { tx.Rollback(ctx) })

	return testPostgres{tx}
}

func setupTestCache(ctx context.Context, t *testing.T) (*cache.Cache, redis.Client) {
	testRedisURL := os.Getenv(EnvKeyTestRedisURL)

	redisOpts, err := redis.ParseURL(testRedisURL)
	assert.NoError(t, err)
	cacheFlusher := redis.NewClient(redisOpts)
	t.Cleanup(func() {
		cacheFlusher.FlushDB(ctx)
		cacheFlusher.Close()
	})

	cache, err := cache.New(ctx, cache.Config{RedisURL: testRedisURL})
	require.NoError(t, err)

	return cache, *cacheFlusher
}
