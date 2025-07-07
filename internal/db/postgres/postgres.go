package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxAttempts = 3
	delay       = 100 * time.Millisecond
)

type Postgres interface {
	Ping(ctx context.Context) error
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type postgresConnectionPool struct {
	*pgxpool.Pool
}

type Config struct {
	DSN string

	MaxConns          int32         // min: 20, max: 50
	MinIddleConns     int32         // min: 0, max: 5
	MaxConnLifetime   time.Duration // min: 0, max: 1h
	MaxConnIdleTime   time.Duration // min: 0, max: 10m
	HealthCheckPeriod time.Duration // min: 30s, max: 2m
}

func New(ctx context.Context, config *Config) (Postgres, error) {
	pgxConfig, err := pgxpool.ParseConfig(config.DSN)
	if err != nil {
		return nil, err
	}

	pgxConfig = applyAppConfig(pgxConfig, config)

	pool, err := pgxpool.NewWithConfig(ctx, pgxConfig)
	if err != nil {
		return nil, err
	}

	go func() {
		<-ctx.Done()
		pool.Close()
	}()

	return &postgresConnectionPool{pool}, nil
}

func applyAppConfig(pgxConfig *pgxpool.Config, appConfig *Config) *pgxpool.Config {
	if appConfig.MaxConns < 20 {
		pgxConfig.MaxConns = 20
	} else if appConfig.MaxConns > 50 {
		pgxConfig.MaxConns = 50
	} else {
		pgxConfig.MaxConns = appConfig.MaxConns
	}

	if appConfig.MinIddleConns < 0 {
		pgxConfig.MinIdleConns = 0
	} else if appConfig.MinIddleConns > 5 {
		pgxConfig.MinIdleConns = 5
	} else {
		pgxConfig.MinIdleConns = appConfig.MinIddleConns
	}

	if appConfig.MaxConnLifetime < 0 {
		pgxConfig.MaxConnLifetime = time.Hour
	} else if appConfig.MaxConnLifetime > time.Hour {
		pgxConfig.MaxConnLifetime = time.Hour
	} else {
		pgxConfig.MaxConnLifetime = appConfig.MaxConnLifetime
	}

	pgxConfig.MaxConnLifetimeJitter = pgxConfig.MaxConnLifetime / 10

	if appConfig.MaxConnIdleTime < 0 {
		pgxConfig.MaxConnIdleTime = 0
	} else if appConfig.MaxConnIdleTime > 10*time.Minute {
		pgxConfig.MaxConnIdleTime = 10 * time.Minute
	} else {
		pgxConfig.MaxConnIdleTime = appConfig.MaxConnIdleTime
	}

	if appConfig.HealthCheckPeriod < 30*time.Second {
		pgxConfig.HealthCheckPeriod = 40 * time.Second
	} else if appConfig.HealthCheckPeriod > 2*time.Minute {
		pgxConfig.HealthCheckPeriod = 2 * time.Minute
	} else {
		pgxConfig.HealthCheckPeriod = appConfig.HealthCheckPeriod
	}

	return pgxConfig
}

func withRetry(ctx context.Context, fn func(context.Context) error) error { // TODO: review and refactor, if needed
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		lastErr = err

		if !isRetryableError(err) {
			return err // fatal error
		}

		// exponential backoff (optional)
		sleep := time.Duration(attempt) * delay
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("operation failed after %d attempts: %w", maxAttempts, lastErr)
}

func isRetryableError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgerrcode.SerializationFailure, pgerrcode.DeadlockDetected:
			return true
		}
	}
	return false
}

func isNotFoundError(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

func isNotUniqueError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}
