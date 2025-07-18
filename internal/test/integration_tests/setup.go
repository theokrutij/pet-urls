package integration_tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	redisx "github.com/theokrutij/pet-urls/internal/cache/redis"
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

func setupTestRedis(ctx context.Context, t *testing.T) redisx.Redis {
	redisClient, err := redisx.New(ctx, redisx.Config{RedisURL: os.Getenv(EnvKeyTestRedisURL)})
	require.NoError(t, err)

	t.Cleanup(func() {
		redisClient.FlushDB(ctx)
	})

	return redisClient
}
