package http

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/theokrutij/pet-urls/internal/services/auth"

	xcontext "github.com/theokrutij/pet-urls/internal/context"
)

// ------- HTTP-specific middleware -------

func maxBodyMiddleware(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodySizeBytes)

		next.ServeHTTP(w, r)
	}

	return http.HandlerFunc(f)
}

func timeoutMiddleware(next http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ------- Logging middleware --------

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

		s.logger.Info().
			Str("method", r.Method).
			Str("path", r.URL.String()).
			Int("status", sc.status).
			Dur("processing time, ms", time.Since(start)).
			Msg("HTTP request done")
	})
}

// ------- Tracing middleware -------

func (s *server) requestIDMiddleware(next http.Handler) http.Handler {
	f := func(w http.ResponseWriter, r *http.Request) {
		requestID, err := generateRequestID()
		if err != nil {
			s.logger.Error().Msg("generating UUID")
			writeResponseAsJSON(w, "internal", 500)
		}

		ctx := xcontext.ContextWithRequestID(r.Context(), requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}

	return http.HandlerFunc(f)
}

func generateRequestID() (string, error) {
	u, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// ------- Auth middleware -------

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

		ctx := xcontext.ContextWithUserID(r.Context(), userID)
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
