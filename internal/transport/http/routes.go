package http

func (s *server) initializeRoutes() {
	// api
	s.router.HandleFunc("GET /api/v1/health", s.handleHealthcheck)
	s.router.HandleFunc("POST /api/v1/short", s.handleGenerateURLToken)

	// TODO:
	// s.router.Handle("POST /api/v1/short/my", s.requiresAuthMiddleware(s.handleCreateTokenWithOwner)
	// s.router.Handle("DELETE /api/v1/short/my/{token}", s.requiresAuthMiddleware(s.handleDeleteToken))

	// auth
	s.router.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	s.router.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	s.router.HandleFunc("POST /api/v1/auth/register", s.handleRegistration)
	s.router.HandleFunc("POST /api/v1/auth/refresh", s.handleTokenRefresh)

	//redirects
	s.router.HandleFunc("GET /{code}", s.redirectToOriginalURL())
}
