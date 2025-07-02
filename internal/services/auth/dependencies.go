package auth

import (
	"context"
)

type repository interface {
	// HealthCheck checks that repository is available.
	// If healthy, returns nil.
	HealthCheck(ctx context.Context) error
	/*
		Saves new user

		If successful, returns UserID.
		If login already exists, isNotUniqueError(err) = true
	*/
	SaveUser(ctx context.Context, user UserInRepo) (UserID, error)
	/*
		Retrieves user

		If login does not exist, isNotFoundError(err) = true
	*/
	GetUser(ctx context.Context, login string) (UserInRepo, error)
	/*
		Saves new refresh token
	*/
	SaveRefreshToken(ctx context.Context, token RefreshTokenInRepo) error
	/*
		Retrieves refresh token

		If tokenHash does not exist, isNotFoundError(err) = true
	*/
	GetRefreshToken(ctx context.Context, tokenHash TokenHash) (RefreshTokenInRepo, error)
	/*
		Deletes refresh token

		If tokenHash does not exist, returns nil error
	*/
	DeleteRefreshToken(ctx context.Context, tokenHash TokenHash) error
}
