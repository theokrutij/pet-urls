package auth

import (
	"context"
)

type repository interface {
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
	GetRefreshToken(ctx context.Context, tokenHash TokenHash) (RefreshTokenInRepo, error) // TODO rename to SaveRefreshToken
	/*
		Deletes refresh token

		If tokenHash does not exist, returns nil error
	*/
	DeleteRefreshToken(ctx context.Context, tokenHash TokenHash) error
}
