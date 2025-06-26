package main

import (
	"log/slog"

	"github.com/theokrutij/pet-urls/internal/cache"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	httpx "github.com/theokrutij/pet-urls/internal/transport/http"
)

type AppConfig struct {
	serverConfig *httpx.Config
	dbConfig     *postgres.Config
	cacheConfig  *cache.Config
}

func loadConfig() AppConfig {
	// Load configuration from a file or environment variables
	// This is a placeholder function. Actual implementation will depend on the configuration management strategy.
	return AppConfig{
		serverConfig: &httpx.Config{
			Debug: true,
			// Port:         8080,
			PathToAssets: "/opt/pet_url/assets",
			Logger:       slog.Default(),
		},
		dbConfig: &postgres.Config{
			DSN: "postgres://postgres:dev-password@localhost:5432/postgres",
		},
		cacheConfig: &cache.Config{
			Addr: "localhost:6379",
		},
	}
}
