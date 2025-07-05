package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/theokrutij/pet-urls/internal/services/auth"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

const (
	// timeouts
	readHeaderTimeout = 2 * time.Second
	readTimeout       = 5 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
	handleTimeout     = 2 * time.Second

	// request size limit
	maxBodySizeBytes = 1 << 20 // 1 mb
)

type server struct {
	s     *http.Server
	debug bool

	shortener shortener.Service
	auth      auth.Service

	router *http.ServeMux
	logger zerolog.Logger
}

type Config struct {
	Debug bool   // default=false
	Port  int    // default=80
	Host  string // default=localhost
}

func NewServer(shortener shortener.Service, auth auth.Service, logger zerolog.Logger, config Config) (*server, error) {
	server := &server{
		s:         &http.Server{},
		shortener: shortener,
		auth:      auth,
		logger:    logger,
	}

	// Initialize the router
	server.router = http.NewServeMux()
	server.initializeRoutes()

	// Middleware
	server.s.Handler = chain(
		server.router,
		server.requestIDMiddleware,
		server.loggingMiddleware,
	)

	// Config
	server = applyConfig(server, config)

	return server, nil
}

func applyConfig(server *server, config Config) *server {
	// debug
	server.debug = config.Debug

	if !server.debug {
		// timeouts
		server.s.ReadHeaderTimeout = readHeaderTimeout
		server.s.ReadTimeout = readTimeout
		server.s.WriteTimeout = writeTimeout
		server.s.IdleTimeout = idleTimeout

		// HTTP-specific middleware
		server.s.Handler = chain(
			server.s.Handler,
			timeoutMiddleware(handleTimeout), // hard timeout in case handler doesn't honor context properly
			maxBodyMiddleware,                // body size limit
		)
	}

	// address
	var port int
	if config.Port == 0 {
		port = 80
	} else {
		port = config.Port
	}
	server.s.Addr = fmt.Sprintf("%s:%d", config.Host, port)

	return server
}

func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
}

func (s *server) Start() error {
	s.logger.Info().Msg(fmt.Sprintf("Listening on %s", s.s.Addr))
	err := s.s.ListenAndServe()
	return err
}

func (s *server) Shutdown(ctx context.Context) error {
	s.logger.Info().Msg("Shutting down")
	err := s.s.Shutdown(ctx)
	return err
}
