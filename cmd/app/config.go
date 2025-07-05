package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/theokrutij/pet-urls/internal/cache"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	httpx "github.com/theokrutij/pet-urls/internal/transport/http"
)

type AppConfig struct {
	debug        bool
	serverConfig *httpx.Config
	dbConfig     *postgres.Config
	cacheConfig  *cache.Config
}

// -------- ENV variable keys -------
const (
	EnvKeyPORT        = "PORT"
	EnvKeyPostgresDSN = "POSTGRES_DSN"
	EnvKeyRedisURL    = "REDIS_URL"
	EnvKeyJwtKey      = "JWT_KEY"
)

func loadConfig() (*AppConfig, error) {
	// CLI flags
	debug := flag.Bool("debug", false, "Run in debug mode")
	flag.Parse()

	// ENV variables
	portEnv, ok := os.LookupEnv(EnvKeyPORT)
	if !ok {
		return nil, fmt.Errorf("%s env key missing", EnvKeyPORT)
	}
	postgresDSN, ok := os.LookupEnv(EnvKeyPostgresDSN)
	if !ok {
		return nil, fmt.Errorf("%s env key missing", EnvKeyPostgresDSN)
	}
	redisURL, ok := os.LookupEnv(EnvKeyRedisURL)
	if !ok {
		return nil, fmt.Errorf("%s env key missing", EnvKeyRedisURL)
	}

	port, err := strconv.Atoi(portEnv)
	if err != nil {
		return nil, fmt.Errorf("%s env key must be integer", EnvKeyPORT)
	}

	return &AppConfig{
		debug: *debug,
		serverConfig: &httpx.Config{
			Debug: *debug,
			Port:  port,
		},
		dbConfig: &postgres.Config{
			DSN: postgresDSN,
		},
		cacheConfig: &cache.Config{
			Addr: redisURL,
		},
	}, nil
}

func loadJWTKey() ([]byte, error) {
	jwtKey, ok := os.LookupEnv(EnvKeyJwtKey)
	if !ok {
		return nil, fmt.Errorf("%s env key missing", EnvKeyJwtKey)
	}
	return []byte(jwtKey), nil
}
