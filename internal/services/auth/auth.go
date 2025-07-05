package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	xcontext "github.com/theokrutij/pet-urls/internal/context"
)

var (
	ErrInvalidLogin       = errors.New("invalid login")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrLoginNotUnique     = errors.New("login not unique")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")

	// used internally to wrap jwt-specific error
	errJWTExpired = errors.New("JWT expired")
)

type UserID []byte
type PasswordHash []byte
type AccessToken []byte
type RefreshToken []byte

type TokenHash []byte

type RefreshTokenInRepo struct {
	Hash      TokenHash
	UserID    UserID
	ExpiresAt time.Time

	// computed in repository
	RevokedAt *time.Time
}

type UserInRepo struct {
	ID           UserID
	Login        string
	PasswordHash PasswordHash
}

type auth struct {
	repo    repository
	keyFunc func() []byte
	logger  zerolog.Logger

	// config parameters
	refreshTokenTTL time.Duration
	accessTokenTTL  time.Duration
}

type Config struct {
	RefreshTokenTTL time.Duration // default: 7 days
	AccessTokenTTL  time.Duration // default: 15 minutes
}

func New(repo repository, config Config, kf func() []byte, logger zerolog.Logger) Service {
	a := &auth{
		repo:    repo,
		keyFunc: kf,
		logger:  logger,
	}
	a = applyConfig(a, config)

	return a
}

func applyConfig(a *auth, c Config) *auth {
	if c.RefreshTokenTTL == 0 {
		a.refreshTokenTTL = 7 * 24 * time.Hour
	} else {
		a.refreshTokenTTL = c.RefreshTokenTTL
	}

	if c.AccessTokenTTL == 0 {
		a.accessTokenTTL = 15 * time.Minute
	} else {
		a.accessTokenTTL = c.AccessTokenTTL
	}

	return a
}

func (a *auth) HealthCheck(ctx context.Context) error {
	logger := a.loggerWithRequestID(ctx)
	if err := a.repo.HealthCheck(ctx); err != nil {
		logger.Error().
			Err(err).
			Msg("repo healthcheck fail")
		return fmt.Errorf("auth, repo healthcheck fail")
	}
	return nil
}

func (a *auth) Register(ctx context.Context, login, password string) error {
	logger := a.loggerWithRequestID(ctx)
	if len(login) > 64 {
		logger.Info().
			Str("login", login).
			Msg("Register: login too long")
		return fmt.Errorf("auth, registering: %w", ErrInvalidLogin)
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		logger.Info().
			Msg("Register: invalid password")
		return fmt.Errorf("auth, hashing password: %w", ErrInvalidPassword)
	}
	userID, err := a.repo.SaveUser(ctx, UserInRepo{Login: login, PasswordHash: passwordHash})
	if isNotUniqueError(err) {
		logger.Info().
			Str("login", login).
			Msg("Register: login already exists")
		return fmt.Errorf("auth, saving user: %w", ErrLoginNotUnique)
	} else if err != nil {
		logger.Error().
			Str("login", login).
			Msg("Register: failed to save use in repo")
		return fmt.Errorf("auth, saving user: repo failure")
	}

	logger.Info().
		Str("userID", string(userID)).
		Msg("Register: success")

	return nil
}

type NotUniqueError interface {
	error
	NotUnique() bool
}

func isNotUniqueError(err error) bool {
	var nuErr NotUniqueError
	return errors.As(err, &nuErr) && nuErr.NotUnique()
}

func (a *auth) Login(ctx context.Context, login, password string) (RefreshToken, AccessToken, error) {
	logger := a.loggerWithRequestID(ctx)
	user, err := a.repo.GetUser(ctx, login)
	if isNotFoundError(err) {
		logger.Warn().
			Str("login", login).
			Msg("Attempting to log in with non-existing login")
		return nil, nil, fmt.Errorf("auth, fetching user from repo: %w", ErrInvalidCredentials)
	} else if err != nil {
		logger.Error().
			Err(err).
			Msg("Login: failed to fetch login from repo")
		return nil, nil, fmt.Errorf("auth, fetching user from repo failed")
	}

	if !passwordMatchesHash(password, user.PasswordHash) {
		logger.Warn().
			Str("login", login).
			Msg("Attempting to log in with incorrect password")
		return nil, nil, fmt.Errorf("auth, validating password: %w", ErrInvalidCredentials)
	}

	refreshToken, err := a.issueRefreshToken(ctx, user.ID)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("Login: failed to issue refresh token")
		return nil, nil, fmt.Errorf("auth, issuing refresh token: %w", err)
	}

	accessToken, err := a.issueAccessToken(user.ID)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("Login: failed to issue access token")
		return nil, nil, fmt.Errorf("auth, issuing access token: %w", err)
	}

	logger.Info().
		Str("login", login).
		Msg("Login: success")

	return refreshToken, accessToken, nil
}

type NotFoundError interface {
	error
	NotFound() bool
}

func isNotFoundError(err error) bool {
	var nfErr NotFoundError
	return errors.As(err, &nfErr) && nfErr.NotFound()
}

func (a *auth) issueRefreshToken(ctx context.Context, userID UserID) (RefreshToken, error) {
	tokenUUID, err := uuid.NewRandom()
	if err != nil { // crypto/rand failure, extremely rare
		return nil, err
	}

	refreshToken := RefreshToken(tokenUUID[:])

	tokenModel := RefreshTokenInRepo{
		Hash:      hashToken(refreshToken),
		UserID:    userID,
		ExpiresAt: time.Now().UTC().Add(a.refreshTokenTTL),
	}
	if err := a.repo.SaveRefreshToken(ctx, tokenModel); err != nil {
		return nil, fmt.Errorf("saving new refresh token to repo")
	}

	return refreshToken, nil
}

func (a *auth) issueAccessToken(userID UserID) (AccessToken, error) {
	claims := claims{
		UserID:    userID,
		ExpiresAt: time.Now().Add(a.accessTokenTTL),
	}
	accessToken, err := generateJWT(claims, a.keyFunc)
	if err != nil {
		return nil, fmt.Errorf("generating JWT")
	}

	return accessToken, nil
}

type claims struct {
	UserID    UserID
	ExpiresAt time.Time
}

func (a *auth) Logout(ctx context.Context, tokenCandidate []byte) error {
	logger := a.loggerWithRequestID(ctx)
	tokenHash := hashToken(tokenCandidate)
	err := a.repo.RevokeRefreshToken(ctx, tokenHash)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("Logout: failed to revoke refresh token")
		return fmt.Errorf("auth, revoking refresh token: internal")
	}

	logger.Info().
		Msg("Logout: success")

	return nil
}

func (a *auth) Refresh(ctx context.Context, tokenCandidate []byte) (AccessToken, error) {
	logger := a.loggerWithRequestID(ctx)

	tokenHash := hashToken(tokenCandidate)

	refreshToken, err := a.repo.GetRefreshToken(ctx, tokenHash)
	if isNotFoundError(err) {
		logger.Warn().
			Msg("Refresh: attempted to refresh access token with non-existing token")
		return nil, fmt.Errorf("auth, fetching refresh token: %w", ErrInvalidToken)
	} else if err != nil {
		logger.Error().
			Err(err).
			Msg("Refresh: failed to fetch refresh token from repo")
		return nil, fmt.Errorf("auth, fetching refresh token failed")
	}

	if refreshToken.RevokedAt != nil {
		logger.Warn().
			Msg("Refresh: attempted to refresh access token with revoked token")
		return nil, fmt.Errorf("auth, refreshing token: %w", ErrInvalidToken)
	}

	if refreshToken.ExpiresAt.Before(time.Now()) {
		logger.Warn().
			Msg("Refresh: attempted to refresh access token with expired token")
		return nil, fmt.Errorf("auth, refreshing token: %w", ErrInvalidToken)
	}

	accessToken, err := a.issueAccessToken(refreshToken.UserID)
	if err != nil {
		logger.Error().
			Err(err).
			Msg("Refresh: failed to issue access token")
		return nil, fmt.Errorf("auth, issuing access token: %w", err)
	}

	logger.Info().
		Msg("Refresh: success")

	return accessToken, nil
}

func (a *auth) Authenticate(ctx context.Context, tokenCandidate []byte) (UserID, error) {
	logger := a.loggerWithRequestID(ctx)

	userID, err := parseJWT(string(tokenCandidate), a.keyFunc)
	if errors.Is(err, errJWTExpired) {
		logger.Warn().
			Msg("Authenticate: attempted to authenticate with expired JWT")
		return nil, fmt.Errorf("auth, validating access token: %w", ErrInvalidToken)
	} else if err != nil {
		logger.Warn().
			Msg("Authenticate: attempted to authenticate with invalid JWT")
		return nil, fmt.Errorf("auth, validating access token: :%w", ErrInvalidToken)
	}

	logger.Info().
		Msg("Authenticate: success")

	return userID, nil
}

func (a *auth) RefreshTokenTTL() time.Duration { return a.refreshTokenTTL }

// ------- Context utils -------

func (a *auth) loggerWithRequestID(ctx context.Context) zerolog.Logger {
	requestID, ok := xcontext.RequestID(ctx)
	if !ok {
		a.logger.Error().Msg("no requestID in context")
		return a.logger
	}

	return a.logger.With().Str("request_id", requestID).Logger()
}
