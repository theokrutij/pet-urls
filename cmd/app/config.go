package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/theokrutij/pet-urls/internal/cache/redis"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	httpx "github.com/theokrutij/pet-urls/internal/transport/http"
)

type AppConfig struct {
	debug        bool
	serverConfig httpx.Config
	pgConfig     postgres.Config
	cacheConfig  redis.Config
}

// -------- ENV variable keys -------
const (
	// app
	EnvKeyPort     = "PORT" // required
	EnvKeyLogLevel = "LOG_LEVEL"

	// postgres
	EnvKeyPostgresDSN                      = "POSTGRES_DSN" // required
	EnvKeyPostgresMaxConnLifetimeSeconds   = "POSTGRES_MAX_CONN_LIFETIME_SECONDS"
	EnvKeyPostgresMaxConnIdleTimeSeconds   = "POSTGRES_MAX_CONN_IDLE_TIME_SECONDS"
	EnvKeyPostgresMaxConns                 = "POSTGRES_MAX_CONNS"
	EnvKeyPostgresMinConns                 = "POSTGRES_MIN_CONNS"
	EnvKeyPostgresMinIdleConns             = "POSTGRES_MIN_IDLE_CONNS"
	EnvKeyPostgresHealthCheckPeriodSeconds = "POSTGRES_HEALTHCHECK_PERIOD_SECONDS"

	// redis
	EnvKeyRedisURL                    = "REDIS_URL" // required
	EnvKeyRedisPoolSize               = "REDIS_POOL_SIZE"
	EnvKeyRedisMinIdleConns           = "REDIS_MIN_IDLE_CONNS"
	EnvKeyRedisMaxIdleConns           = "REDIS_MAX_IDLE_CONNS"
	EnvKeyRedisConnMaxIdleTimeSeconds = "REDIS_CONN_MAX_IDLE_TIME_SECONDS"

	// secrets
	EnvKeyJwtKeySecret = "SECRET_JWT_KEY"
)

func loadConfig() (*AppConfig, error) {
	// app config
	port, err := lookupEnvInt(EnvKeyPort)
	if err != nil {
		return nil, err
	}
	logLevel := os.Getenv(EnvKeyLogLevel)
	debug := logLevel == "DEBUG"

	// postgres config
	pgConfig, err := loadPostgresConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("loading postgres config: %w", err)
	}

	// redis config
	redisConfig, err := loadRedisConfigFromEnv()
	if err != nil {
		return nil, fmt.Errorf("loading redis config: %w", err)
	}

	return &AppConfig{
		debug: debug,
		serverConfig: httpx.Config{
			Debug: debug,
			Port:  port,
		},
		pgConfig:    *pgConfig,
		cacheConfig: *redisConfig,
	}, nil
}

func loadPostgresConfigFromEnv() (*postgres.Config, error) {
	var pgConfig = new(postgres.Config)

	var ok bool
	pgConfig.DSN, ok = os.LookupEnv(EnvKeyPostgresDSN)
	if !ok {
		return nil, fmt.Errorf("%s key is required", EnvKeyPostgresDSN)
	}

	for _, config := range []struct {
		envKeyInt string
		field     *int
	}{
		{EnvKeyPostgresMaxConnLifetimeSeconds, &pgConfig.MaxConnLifetimeSeconds},
		{EnvKeyPostgresMaxConnIdleTimeSeconds, &pgConfig.MaxConnIdleTimeSeconds},
		{EnvKeyPostgresMaxConns, &pgConfig.MaxConns},
		{EnvKeyPostgresMinConns, &pgConfig.MinConns},
		{EnvKeyPostgresMinIdleConns, &pgConfig.MinIdleConns},
		{EnvKeyPostgresHealthCheckPeriodSeconds, &pgConfig.HealthCheckPeriodSeconds},
	} {
		val, err := lookupEnvInt(config.envKeyInt)
		if err != nil {
			return nil, err
		}
		*config.field = val
	}

	return pgConfig, nil
}

func loadRedisConfigFromEnv() (*redis.Config, error) {
	var redisConfig = new(redis.Config)

	var ok bool
	redisConfig.RedisURL, ok = os.LookupEnv(EnvKeyRedisURL)
	if !ok {
		return nil, fmt.Errorf("%s key is required", EnvKeyRedisURL)
	}

	for _, config := range []struct {
		envKeyInt string
		field     *int
	}{
		{EnvKeyRedisPoolSize, &redisConfig.PoolSize},
		{EnvKeyRedisPoolSize, &redisConfig.PoolSize},
		{EnvKeyRedisMinIdleConns, &redisConfig.MinIdleConns},
		{EnvKeyRedisMaxIdleConns, &redisConfig.MaxIdleConns},
		{EnvKeyRedisConnMaxIdleTimeSeconds, &redisConfig.ConnMaxIdleTimeSeconds},
	} {
		val, err := lookupEnvInt(config.envKeyInt)
		if err != nil {
			return nil, err
		}
		*config.field = val

	}

	return redisConfig, nil
}

// lookupEnvInt(key) returns 0 if key env variable is not set
// or an error if it cannot be converted to int.
func lookupEnvInt(key string) (int, error) {
	valStr, ok := os.LookupEnv(key)
	if !ok {
		return 0, nil
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return 0, fmt.Errorf("%s ENV variable must be of type int", key)
	}
	return val, nil
}

func loadJWTKey() ([]byte, error) {
	jwtKey, ok := os.LookupEnv(EnvKeyJwtKeySecret)
	if !ok {
		return nil, fmt.Errorf("%s env key missing", EnvKeyJwtKeySecret)
	}
	return []byte(jwtKey), nil
}
