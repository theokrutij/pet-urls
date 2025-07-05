package auth

import (
	"context"
	"time"
)

type Service interface {
	// Healtcheck runs healthcheck of auth service and all its dependencies.
	// If healthy, returns nil.
	HealthCheck(ctx context.Context) error
	/*
		Creates new user represented by userID.

		If successful, returns nil.
		If login cannot be used, returns ErrInvalidLogin.
		If password cannot be used, returns ErrInvalidPassword.
		If login is not unique, returns ErrLoginNotUnique
	*/
	Register(ctx context.Context, login, password string) error
	/*
		Creates new user session represented by refreshToken.

		If successful, returns RefreshToken and AccessToken.
		If credentials are invalid, returns ErrInvalidCredentials.
	*/
	Login(ctx context.Context, login, password string) (RefreshToken, AccessToken, error)

	/*
		Invalidates user session represented by the refreshToken.

		If error is nil, refresh token cannot be used anymore.
	*/
	Logout(ctx context.Context, tokenCandidate []byte) error

	/*
		Issues an access token for user sesion represented by the refreshToken.

		If successful, returns accessToken.
		If tokenCandidate cannot be used to issue an accessToken, returns ErrInvalidToken.
	*/
	Refresh(ctx context.Context, tokenCandidate []byte) (AccessToken, error)

	/*
		Authenticates user with accessToken.

		If accessToken was issued by a user session that is active, returns associated userID.
		If accessToken is not valid, returns ErrInvalidToken.
	*/
	Authenticate(ctx context.Context, tokenCandidate []byte) (userID UserID, err error)

	// RefreshTokenTTL returns the refresh token TTL.
	RefreshTokenTTL() time.Duration

	// TODO:
	// ChangePassword (probably requires email capabilities)
	// DeleteUser
}
