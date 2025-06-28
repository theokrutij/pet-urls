package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/theokrutij/pet-urls/internal/services/auth"
)

type statusCapture struct {
	http.ResponseWriter
	status int
}

func (w *statusCapture) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusCapture) Write(b []byte) (int, error) {
	return w.ResponseWriter.Write(b)
}

func (s *server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sc := &statusCapture{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(sc, r)

		s.logger.Info(fmt.Sprintf("%s %s -> %d %s", r.Method, r.URL, sc.status, time.Since(start)))
	})
}

func timeoutMiddleware(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *server) requiresAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := tokenFromHeader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		userID, err := s.auth.Authenticate(r.Context(), []byte(token))
		if errors.Is(err, auth.ErrTokenExpired) {
			http.Error(w, "Token expired", http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		ctx := contextWithUserID(r.Context(), userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func tokenFromHeader(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", errors.New("authorization header required")
	}

	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", errors.New("bearer schema required")
	}

	token := strings.TrimPrefix(authHeader, "Bearer ")

	if token == "" {
		return "", errors.New("token is empty")
	}

	return token, nil
}

//TODO: max body middleware
