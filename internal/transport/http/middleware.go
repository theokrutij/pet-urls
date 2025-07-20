package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/theokrutij/pet-urls/internal/services/auth"

	xcontext "github.com/theokrutij/pet-urls/internal/context"
)

func chain(h http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		h = middlewares[i](h)
	}
	return h
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

// ------- Logging middleware --------

func (s *server) observabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &writerWrapper{ResponseWriter: w}

		next.ServeHTTP(ww, r)
		ww.Flush()

		duration := time.Since(start)

		requestID, ok := xcontext.RequestID(r.Context())
		if !ok {
			s.logger.Fatal().Msg("requestID not in context")
		}

		// log request completion
		s.logger.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("path", r.URL.String()).
			Int("status", ww.status).
			Dur("duration_ms", duration).
			Msg("HTTP request completed")

		// update metrics
		if s.metrics != nil && r.Pattern != "" {
			path := patternhWithoutMethod(r.Pattern)
			statusStr := strconv.Itoa(ww.status)

			s.metrics.observeRequest(r.Method, path, statusStr, duration.Seconds())
		}
	})
}

// patternhWithoutMethod trims HTTP method from http.Request.Pattern
func patternhWithoutMethod(pattern string) string {
	beforeSpace, afterSpace, patternHasSpace := strings.Cut(pattern, " ")
	if patternHasSpace {
		return afterSpace
	} else {
		return beforeSpace
	}

}

// ------- HTTP-specific middleware -------

func maxBodyMiddleware(maxBodySizeBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		f := func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBodySizeBytes)

			next.ServeHTTP(w, r)
		}
		return http.HandlerFunc(f)
	}
}

type writerWrapper struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	buffer      bytes.Buffer
	mu          sync.Mutex
	aborted     bool
}

func (ww *writerWrapper) WriteHeader(status int) {
	ww.mu.Lock()
	defer ww.mu.Unlock()

	if ww.aborted || ww.wroteHeader {
		return
	}
	ww.status = status
	ww.wroteHeader = true
}

func (ww *writerWrapper) Write(b []byte) (int, error) {
	ww.mu.Lock()
	defer ww.mu.Unlock()

	if ww.aborted {
		return 0, nil
	}

	if !ww.wroteHeader {
		ww.WriteHeader(200)
	}

	return ww.buffer.Write(b)
}

func (ww *writerWrapper) Flush() {
	ww.ResponseWriter.WriteHeader(ww.status)
	ww.ResponseWriter.Write(ww.buffer.Bytes())
}

func (ww *writerWrapper) Abort() {
	ww.mu.Lock()
	defer ww.mu.Unlock()

	ww.aborted = true
	ww.buffer.Reset()
}

func timeoutMiddleware(timeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			done := make(chan struct{})

			ww := &writerWrapper{ResponseWriter: w}
			go func() {
				next.ServeHTTP(ww, r.WithContext(ctx))
				close(done)
			}()

			select {
			case <-ctx.Done():
				ww.Abort()
				w.Header().Set("Retry-After", "5")
				http.Error(w, "Server timeout", http.StatusServiceUnavailable)
			case <-done:
				ww.Flush()
			}
		})
	}
}

// ------- Auth middleware -------

func (s *server) requiresAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, err := tokenFromHeader(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}

		userID, err := s.auth.Authenticate(r.Context(), token)
		if errors.Is(err, auth.ErrInvalidToken) {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		} else if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
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
