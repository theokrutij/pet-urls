package http

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/theokrutij/pet-urls/internal/services/auth"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

type server struct {
	s     *http.Server
	debug bool

	shortener shortener.Service
	auth      auth.Service

	router       *http.ServeMux
	logger       *slog.Logger
	pathToStatic string
}

type Config struct {
	Debug        bool         // default=false
	Port         int          // default=80
	Host         string       // default=localhost
	PathToAssets string       // default=no static
	Logger       *slog.Logger // default=no logging
}

func NewServer(shortener shortener.Service, auth auth.Service, config Config) (*server, error) {
	server := &server{
		s:         &http.Server{},
		shortener: shortener,
		auth:      auth,
	}

	// Initialize the router
	server.router = http.NewServeMux()
	server.s.Handler = server.router
	server.initializeRoutes()

	// Config
	server = applyConfig(server, config)

	return server, nil
}

func applyConfig(server *server, config Config) *server {
	// debug
	server.debug = config.Debug
	if !server.debug {
		server.s.Handler = timeoutMiddleware(server.s.Handler)
		server.s.MaxHeaderBytes = 1 << 20 //1MB
		server.s.ReadHeaderTimeout = 2 * time.Second
		server.s.ReadTimeout = 5 * time.Second
		server.s.WriteTimeout = 10 * time.Second
		server.s.IdleTimeout = 60 * time.Second
	}

	// address
	var port int
	if config.Port == 0 {
		port = 80
	} else {
		port = config.Port
	}
	server.s.Addr = fmt.Sprintf("%s:%d", config.Host, port)

	// logger
	if config.Logger != nil {
		server.logger = config.Logger
		server.s.Handler = server.loggingMiddleware(server.s.Handler)
	}

	return server
}

func (s *server) Start() error {
	s.logger.Info(fmt.Sprintf("Listening on %s", s.s.Addr))
	err := s.s.ListenAndServe()
	return err
}

func (s *server) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down")
	err := s.s.Shutdown(ctx)
	return err
}
