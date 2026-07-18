package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"

	xcontext "github.com/theokrutij/pet-urls/internal/context"
)

const (
	defaultRefreshTokenTTL = time.Hour * 24 * 7
	defaultAccessTokenTTL  = time.Minute * 15
)

var (
	ErrInvalidLogin       = errors.New("invalid login")
	ErrInvalidPassword    = errors.New("invalid password")
	ErrLoginNotUnique     = errors.New("login not unique")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrInvalidToken       = errors.New("invalid token")
)

type UserID []byte

// converts UserID into base64 url-encoded string
func (id UserID) toBase64() string {
	return base64.RawURLEncoding.EncodeToString(id)
}

// decodes base64 url-encoded string into UserID
func userIDfromBase64(s string) (UserID, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

type PasswordHash []byte
type AccessToken string
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

	// config parameters
	refreshTokenTTL time.Duration
	accessTokenTTL  time.Duration
	logger          zerolog.Logger
}

type Dependencies struct {
	Repo    repository
	KeyFunc func() []byte
}

type Config struct {
	RefreshTokenTTL time.Duration  // default: 7 days
	AccessTokenTTL  time.Duration  // default: 15 minutes
	Logger          zerolog.Logger // default: no logging
}

func New(deps Dependencies, config Config) Service {
	if deps.Repo == nil || deps.KeyFunc == nil {
		panic("Got nil instead of dependency")
	}

	a := &auth{
		repo:    deps.Repo,
		keyFunc: deps.KeyFunc,
	}
	a = applyConfig(a, config)

	return a
}

func applyConfig(a *auth, c Config) *auth {
	if c.RefreshTokenTTL == 0 {
		a.refreshTokenTTL = defaultRefreshTokenTTL
	} else {
		a.refreshTokenTTL = c.RefreshTokenTTL
	}

	if c.AccessTokenTTL == 0 {
		a.accessTokenTTL = defaultAccessTokenTTL
	} else {
		a.accessTokenTTL = c.AccessTokenTTL
	}

	a.logger = c.Logger

	return a
}

// ------- Public methods ------

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
		Str("user_id", userID.toBase64()).
		Msg("Register: success")
	return nil
}

func (a *auth) Login(ctx context.Context, login, password string) (RefreshToken, AccessToken, error) {
	logger := a.loggerWithRequestID(ctx)

	user, err := a.repo.GetUser(ctx, login)
	if isNotFoundError(err) {
		logger.Warn().
			Str("login", login).
			Msg("Attempting to log in with non-existing login")
		return nil, "", fmt.Errorf("auth, fetching user from repo: %w", ErrInvalidCredentials)
	} else if err != nil {
		logger.Error().
			Msg("Login: failed to fetch login from repo")
		return nil, "", fmt.Errorf("auth, fetching user from repo failed")
	}

	if !passwordMatchesHash(password, user.PasswordHash) {
		logger.Warn().
			Str("login", login).
			Msg("Attempting to log in with incorrect password")
		return nil, "", fmt.Errorf("auth, validating password: %w", ErrInvalidCredentials)
	}

	refreshToken, err := a.issueRefreshToken(ctx, user.ID)
	if err != nil {
		logger.Error().
			Msg("Login: failed to issue refresh token")
		return nil, "", fmt.Errorf("auth, issuing refresh token: %w", err)
	}

	accessToken, err := a.issueAccessToken(user.ID)
	if err != nil {
		logger.Error().
			Msg("Login: failed to issue access token")
		return nil, "", fmt.Errorf("auth, issuing access token: %w", err)
	}

	logger.Info().
		Str("login", login).
		Msg("Login: success")
	return refreshToken, accessToken, nil
}

func (a *auth) Logout(ctx context.Context, tokenCandidate []byte) error {
	logger := a.loggerWithRequestID(ctx)
	tokenHash := hashToken(tokenCandidate)
	err := a.repo.RevokeRefreshToken(ctx, tokenHash)
	if err != nil {
		logger.Error().
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
		return "", fmt.Errorf("auth, fetching refresh token: %w", ErrInvalidToken)
	} else if err != nil {
		logger.Error().
			Msg("Refresh: failed to fetch refresh token from repo")
		return "", fmt.Errorf("auth, fetching refresh token failed")
	}

	if refreshToken.RevokedAt != nil {
		logger.Warn().
			Msg("Refresh: attempted to refresh access token with revoked token")
		return "", fmt.Errorf("auth, refreshing token: %w", ErrInvalidToken)
	}

	if refreshToken.ExpiresAt.Before(time.Now()) {
		logger.Warn().
			Msg("Refresh: attempted to refresh access token with expired token")
		return "", fmt.Errorf("auth, refreshing token: %w", ErrInvalidToken)
	}

	accessToken, err := a.issueAccessToken(refreshToken.UserID)
	if err != nil {
		logger.Error().
			Msg("Refresh: failed to issue access token")
		return "", fmt.Errorf("auth, issuing access token: %w", err)
	}

	logger.Info().
		Msg("Refresh: success")

	return accessToken, nil
}

func (a *auth) Authenticate(ctx context.Context, tokenCandidate string) (UserID, error) {
	logger := a.loggerWithRequestID(ctx)

	claims, err := parseJWT(tokenCandidate, a.keyFunc)
	if err != nil {
		logger.Warn().
			Msg("Authenticate: attempted to authenticate with invalid JWT")
		return nil, fmt.Errorf("auth, parsing tokenCandidate: %w", ErrInvalidToken)
	}

	if claims.ExpiresAt.Before(time.Now()) {
		logger.Warn().
			Msg("Authenticate: attempted to authenticate with expired JWT")
		return nil, fmt.Errorf("auth, validation tokenCandidate: %w", ErrInvalidToken)
	}

	userID, err := userIDfromBase64(claims.UserID)
	if err != nil {
		logger.Warn().
			Msg("Authenticate: attempted to authenticate with invalid JWT claim encoding")
		return nil, fmt.Errorf("auth, parsing userID: %w", ErrInvalidToken)
	}

	logger.Info().
		Msg("Authenticate: success")

	return userID, nil
}

func (a *auth) RefreshTokenTTL() time.Duration { return a.refreshTokenTTL }

// ------- Token management utils -------

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
	claims := JWTclaims{
		UserID:    userID.toBase64(),
		ExpiresAt: time.Now().Add(a.accessTokenTTL),
	}
	accessToken, err := generateJWT(claims, a.keyFunc)
	if err != nil {
		return "", fmt.Errorf("generating JWT")
	}

	return AccessToken(accessToken), nil
}

// ------- Error type checkers -------

func isNotUniqueError(err error) bool {
	type NotUniqueError interface {
		error
		NotUnique() bool
	}
	var nuErr NotUniqueError

	return errors.As(err, &nuErr) && nuErr.NotUnique()
}

func isNotFoundError(err error) bool {
	type NotFoundError interface {
		error
		NotFound() bool
	}
	var nfErr NotFoundError

	return errors.As(err, &nfErr) && nfErr.NotFound()
}

// ------- Context utils -------

func (a *auth) loggerWithRequestID(ctx context.Context) zerolog.Logger {
	requestID, ok := xcontext.RequestID(ctx)
	if !ok {
		a.logger.Error().Msg("no requestID in context")
		return a.logger
	}

	return a.logger.With().Str("request_id", requestID).Logger()
}
