package postgres

import (
	"context"
	"fmt"

	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

type shortenerRepository struct {
	p *postgresConnectionPool
}

func NewShortenerRepository(p *postgresConnectionPool) *shortenerRepository {
	return &shortenerRepository{p}
}

func (s *shortenerRepository) HealthCheck(ctx context.Context) error {
	return s.p.Ping(ctx)
}

func (s *shortenerRepository) SaveToken(ctx context.Context, token shortener.URLToken) error {
	const query = `
		INSERT INTO url_tokens (token, url, valid_until, owner_id)
		VALUES ($1, $2, $3, $4)
	`
	err := withRetry(ctx, func(ctx context.Context) error {
		_, err := s.p.Exec(ctx, query, token.Token, token.URL, token.ExpiresAt, token.OwnerID)
		return err
	})
	if isNotUniqueError(err) {
		return fmt.Errorf("postgres, saving token: %w", notUniqueError{err})
	} else if err != nil {
		return fmt.Errorf("postgres, saving token: %w", err)
	}

	return nil
}

type notFoundError struct {
	inner error
}

func (n notFoundError) Error() string {
	return "not found"
}

func (n notFoundError) Unwrap() error {
	return n.inner
}

func (n notFoundError) NotFound() bool {
	return true
}

func (s *shortenerRepository) GetToken(ctx context.Context, tokenStr string) (shortener.URLToken, error) {
	const query = `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	token := shortener.URLToken{Token: shortener.Token(tokenStr)}
	err := withRetry(ctx, func(ctx context.Context) error {
		return s.p.QueryRow(ctx, query, tokenStr).Scan(&token.URL, &token.ExpiresAt, &token.OwnerID)
	})

	if isNotFoundError(err) {
		return token, fmt.Errorf("postgres, fetching token: %w", &notFoundError{err})
	} else if err != nil {
		return token, fmt.Errorf("postgres, fetching token: %w", err)
	}

	return token, nil
}

func (s *shortenerRepository) DeleteToken(ctx context.Context, token string) error {
	const query = `
		DELETE FROM url_tokens
		WHERE token = $1
	`
	return withRetry(ctx, func(ctx context.Context) error {
		_, err := s.p.Exec(ctx, query, token)
		return err
	})
}
