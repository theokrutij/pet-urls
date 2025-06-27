package main

import (
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

func loadConfig() (AppConfig, error) {
	// CLI flags
	debug := flag.Bool("debug", false, "Run in debug mode")

	// ENV variables
	portEnv := os.Getenv("PORT")
	staticPath := os.Getenv("STATIC_PATH")
	postgresDSN := os.Getenv("POSTGRES_DSN")
	redisURL := os.Getenv("REDIS_URL")

	port, err := strconv.Atoi(portEnv)
	if err != nil {
		return AppConfig{}, err
	}

	return AppConfig{
		serverConfig: &httpx.Config{
			Debug:        *debug,
			Port:         port,
			PathToAssets: staticPath,
			Logger:       slog.Default(), // TODO: add log setup
		},
		dbConfig: &postgres.Config{
			DSN: postgresDSN,
		},
		cacheConfig: &cache.Config{
			Addr: redisURL,
		},
	}, nil
}
