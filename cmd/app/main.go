package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog"
	"github.com/theokrutij/pet-urls/internal/cache/redis"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	"github.com/theokrutij/pet-urls/internal/services/auth"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
	httpx "github.com/theokrutij/pet-urls/internal/transport/http"
)

func main() {
	baseLogger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Lifecycle logger
	lifecycleLogger := baseLogger.With().Str("component", "lifecycle").Logger()

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		lifecycleLogger.Fatal().
			Err(err).
			Msg("failed to load config")
	}

	// Context
	appContext, cancelAppContext := context.WithCancel(context.Background())
	defer cancelAppContext()

	// Postgres
	dbInstance, err := postgres.New(appContext, config.pgConfig)
	if err != nil {
		lifecycleLogger.Fatal().
			Err(err).
			Msg("failed to init postgres client")
	}

	// Redis
	config.cacheConfig.Logger = baseLogger.With().Str("component", "redis").Logger()
	redisInstance, err := redis.New(appContext, config.cacheConfig)
	if err != nil {
		lifecycleLogger.Fatal().
			Err(err).
			Msg("failed to init redis client")
	}

	// Server key
	jwtKey, err := loadJWTKey()
	if err != nil {
		lifecycleLogger.Fatal().
			Err(err).
			Msg("failed to load JWT key")
	}

	// Logger config
	zerolog.DurationFieldUnit = time.Millisecond // it is already set as a default value, written out here for documentation
	if config.debug {
		baseLogger = baseLogger.Level(zerolog.DebugLevel)
	} else {
		baseLogger = baseLogger.Level(zerolog.InfoLevel)
	}
	config.serverConfig.Logger = baseLogger.With().Str("component", "http").Logger()

	// Prometheus registry
	config.serverConfig.PromRegistry = prometheus.NewRegistry()

	// Server instance
	server, err := httpx.NewServer(
		httpx.Dependencies{
			Shortener: shortener.New(
				shortener.Dependencies{
					Repo:  postgres.NewShortenerRepository(dbInstance),
					Cache: redis.NewShortenerCache(redisInstance),
				},
				shortener.Config{
					Logger: baseLogger.With().Str("component", "shortener").Logger(),
				},
			),
			Auth: auth.New(
				auth.Dependencies{
					Repo:    postgres.NewAuthRepository(dbInstance),
					KeyFunc: func() []byte { return jwtKey },
				},
				auth.Config{
					Logger: baseLogger.With().Str("component", "auth").Logger(),
				},
			),
		},
		config.serverConfig,
	)
	if err != nil {
		lifecycleLogger.Fatal().
			Err(err).
			Msg("failed to init http server")
	}

	// Errors channel with a buffer, allowing runners to return error after main goroutine exits
	errs := make(chan error, 1)

	go func() {
		errs <- server.Start()
	}()

	// Listen for SIGINT or SIGTERM
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	// Wait either for stop or for either runner to return an error
	select {
	case err := <-errs:
		lifecycleLogger.Fatal().
			Err(err).
			Msg("got error from runners, exiting now")
	case <-stop:
		ctx, cancel := context.WithTimeout(appContext, 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			lifecycleLogger.Error().
				Err(err).
				Msg("error from http server shutdown")
		}
	}

	lifecycleLogger.Info().
		Msg("Exiting now")
}
