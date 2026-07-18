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
	DSN string // required

	MaxConnLifetimeSeconds   int // default: 3600
	MaxConnIdleTimeSeconds   int // default: 0
	MaxConns                 int // default: 4
	MinConns                 int // default: 0
	MinIdleConns             int // default: 0
	HealthCheckPeriodSeconds int // default: 60
}

func New(ctx context.Context, config Config) (Postgres, error) {
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

func applyAppConfig(pgxConfig *pgxpool.Config, appConfig Config) *pgxpool.Config {
	if appConfig.MaxConnLifetimeSeconds == 0 {
		pgxConfig.MaxConnLifetime = 3600 * time.Second
	} else {
		pgxConfig.MaxConnLifetime = time.Duration(appConfig.MaxConnLifetimeSeconds) * time.Second
	}

	pgxConfig.MaxConnLifetimeJitter = pgxConfig.MaxConnLifetime / 10

	pgxConfig.MaxConnIdleTime = time.Duration(appConfig.MaxConnIdleTimeSeconds) * time.Second

	if appConfig.MaxConns == 0 {
		pgxConfig.MaxConns = 4
	} else {
		pgxConfig.MaxConns = int32(appConfig.MaxConns)
	}

	pgxConfig.MinConns = int32(appConfig.MinConns)

	pgxConfig.MinIdleConns = int32(appConfig.MinIdleConns)

	if appConfig.HealthCheckPeriodSeconds == 0 {
		pgxConfig.HealthCheckPeriod = 60 * time.Second
	} else {
		pgxConfig.HealthCheckPeriod = time.Duration(appConfig.HealthCheckPeriodSeconds) * time.Second
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
