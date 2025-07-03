package main

import (
	"errors"
	"flag"
	"log/slog"
	"os"
	"strconv"

	"github.com/theokrutij/pet-urls/internal/cache"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	httpx "github.com/theokrutij/pet-urls/internal/transport/http"
)

type AppConfig struct {
	serverConfig *httpx.Config
	dbConfig     *postgres.Config
	cacheConfig  *cache.Config
}

func loadConfig() (*AppConfig, error) {
	// CLI flags
	debug := flag.Bool("debug", false, "Run in debug mode")
	flag.Parse()

	// ENV variables
	portEnv, ok := os.LookupEnv("PORT")
	if !ok {
		return nil, errors.New("PORT env variable missing")
	}
	postgresDSN, ok := os.LookupEnv("POSTGRES_DSN")
	if !ok {
		return nil, errors.New("POSTGRES_DSN env variable missing")
	}
	redisURL, ok := os.LookupEnv("REDIS_URL")
	if !ok {
		return nil, errors.New("REDIS_URL variable missing")
	}

	port, err := strconv.Atoi(portEnv)
	if err != nil {
		return nil, err
	}

	return &AppConfig{
		serverConfig: &httpx.Config{
			Debug:  *debug,
			Port:   port,
			Logger: slog.Default(), // TODO: add log setup
		},
		dbConfig: &postgres.Config{
			DSN: postgresDSN,
		},
		cacheConfig: &cache.Config{
			Addr: redisURL,
		},
	}, nil
}
