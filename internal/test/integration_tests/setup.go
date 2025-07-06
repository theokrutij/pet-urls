package integration_tests

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/theokrutij/pet-urls/internal/cache"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

// mockPostgres implements the postgres.Postgres interface.
// It uses pgx.Tx instead of a connection pool,
// and defines a Ping method to satisfy the interface.
type mockPostgres struct {
	pgx.Tx
}

func (m mockPostgres) Ping(ctx context.Context) error { return nil }

func setupTestShortener(t *testing.T, tx pgx.Tx) shortener.Service {
	repo := postgres.NewShortenerRepository(mockPostgres{tx})

	cache := cache.New(&cache.Config{Addr: os.Getenv("TEST_REDIS_URL")})
	t.Cleanup(func() {
		if err := getCacheFlusher().FlushDB(context.Background()).Err(); err != nil {
			t.Fatalf("failed to flush Redis: %v", err)
		}
	})

	return shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{Logger: zerolog.Nop()})
}

func beginTx(ctx context.Context, t *testing.T) pgx.Tx {
	testPool, err := getTestPool()
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	t.Cleanup(func() {
		cleanUpCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		tx.Rollback((cleanUpCtx))
	})
	return tx
}

var getTestPool = sync.OnceValues(func() (*pgxpool.Pool, error) {
	return pgxpool.New(context.Background(), os.Getenv("TEST_POSTGRES_DSN"))
})

var getCacheFlusher = sync.OnceValue(func() *redis.Client {
	return redis.NewClient(&redis.Options{Addr: os.Getenv("TEST_REDIS_URL")})
})
