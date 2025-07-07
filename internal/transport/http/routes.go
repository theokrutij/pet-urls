package http

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func (s *server) initializeRoutes(reg *prometheus.Registry) {
	// api
	s.router.HandleFunc("GET /api/v1/health", s.handleHealthcheck)
	s.router.HandleFunc("POST /api/v1/short", s.handleGenerateURLToken)

	s.router.Handle("POST /api/v1/short/my", s.requiresAuthMiddleware(http.HandlerFunc(s.handleCreateTokenWithOwner)))
	s.router.Handle("DELETE /api/v1/short/my/{token}", s.requiresAuthMiddleware(http.HandlerFunc(s.handleDeleteToken)))

	// auth
	s.router.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.router.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.router.HandleFunc("POST /api/v1/auth/register", s.handleRegistration)
	s.router.HandleFunc("POST /api/v1/auth/refresh", s.handleTokenRefresh)

	//redirects
	s.router.HandleFunc("GET /api/v1/{token}", s.handleResolveToken())

	// metrics
	if reg != nil {
		s.router.Handle(("GET /metrics"), promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	}
}
