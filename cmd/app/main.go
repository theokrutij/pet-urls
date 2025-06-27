package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/theokrutij/pet-urls/internal/cache"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	"github.com/theokrutij/pet-urls/internal/services/auth"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
	httpx "github.com/theokrutij/pet-urls/internal/transport/http"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	// Load configuration
	config, err := loadConfig()
	if err != nil {
		return err
	}

	appContext, cancelAppContext := context.WithCancel(context.Background())
	defer cancelAppContext()
	// Init adapters
	dbInstance, err := postgres.New(appContext, config.dbConfig)
	if err != nil {
		return fmt.Errorf("postgres.New: %w", err)
	}
	cacheInstance := cache.New(config.cacheConfig) // Replace with actual cache initialization if needed

	// Initialize the HTTP server with services and config
	server, err := httpx.NewServer(
		shortener.New(
			postgres.NewShortenerRepository(dbInstance),
			cacheInstance,
			shortener.Config{},
		),
		auth.New(
			postgres.NewAuthRepository(dbInstance),
			auth.Config{},
			func() []byte { return []byte("abc") },
		),
		*config.serverConfig,
	)
	if err != nil {
		return err
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
		return err
	case <-stop:
		ctx, cancel := context.WithTimeout(appContext, 5*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("server shudown: %s", err)
		}
	}

	return nil
}
