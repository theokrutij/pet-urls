package postgres

import (
	"context"
	"fmt"

	"github.com/theokrutij/pet-urls/internal/services/auth"
)

type authRepository struct {
	p *postgresConnectionPool
}

type notUniqueError struct {
	inner error
}

func (n notUniqueError) Error() string {
	return "not unique"
}

func (n notUniqueError) Unwrap() error {
	return n.inner
}

func (n notUniqueError) NotUnique() bool {
	return true
}

func NewAuthRepository(p *postgresConnectionPool) *authRepository {
	return &authRepository{p}
}

func (a *authRepository) SaveUser(ctx context.Context, user auth.UserInRepo) (auth.UserID, error) {
	const query = `
		INSERT INTO users (login, password_hash)
		VALUES ($1, $2) 
		RETURNING id 
	`
	var userID auth.UserID
	err := withRetry(ctx, func(ctx context.Context) error {
		return a.p.QueryRow(ctx, query, user.Login, user.PasswordHash).Scan(&userID)
	})
	if isNotUniqueError(err) {
		return nil, &notUniqueError{err}
	} else if err != nil {
		return nil, fmt.Errorf("postgres, saving user: %w", err)
	}

	return userID, nil
}

func (a *authRepository) GetUser(ctx context.Context, login string) (auth.UserInRepo, error) {
	const query = `
		SELECT id, password_hash FROM users
		WHERE login = $1
	`

	user := auth.UserInRepo{Login: login}

	err := withRetry(ctx, func(ctx context.Context) error {
		return a.p.QueryRow(ctx, query, login).Scan(&user.ID, &user.PasswordHash)
	})
	if isNotFoundError(err) {
		return user, fmt.Errorf("postgres, fetching user: %w", &notFoundError{err})
	} else if err != nil {
		return user, fmt.Errorf("postgres, fetching user: %w", err)
	}

	return user, nil
}

func (a *authRepository) SaveRefreshToken(ctx context.Context, token auth.RefreshTokenInRepo) error {
	const query = `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`
	err := withRetry(ctx, func(ctx context.Context) error {
		_, err := a.p.Exec(ctx, query, token.Hash, token.UserID, token.ExpiresAt)
		return err
	})
	if err != nil {
		return fmt.Errorf("postgres, saving refresh token: %w", err)
	}

	return nil
}

func (a *authRepository) GetRefreshToken(ctx context.Context, tokenHash auth.TokenHash) (auth.RefreshTokenInRepo, error) {
	const query = `
		SELECT user_id, expires_at, revoked_at FROM refresh_tokens
		WHERE token_hash = $1
	`

	token := auth.RefreshTokenInRepo{Hash: tokenHash}

	err := withRetry(ctx, func(ctx context.Context) error {
		return a.p.QueryRow(ctx, query, tokenHash).Scan(&token.UserID, &token.ExpiresAt, &token.RevokedAt)
	})
	if isNotFoundError(err) {
		return token, fmt.Errorf("postgres, fetching token: %w", &notFoundError{err})
	} else if err != nil {
		return token, fmt.Errorf("postgres, fetching token: %w", err)
	}

	return token, nil
}

func (a *authRepository) DeleteRefreshToken(ctx context.Context, tokenHash auth.TokenHash) error {
	const query = `
		DELETE FROM refresh_tokens
		WHERE token_hash = $1
	`
	return withRetry(ctx, func(ctx context.Context) error {
		_, err := a.p.Exec(ctx, query, tokenHash)
		return err
	})
}
