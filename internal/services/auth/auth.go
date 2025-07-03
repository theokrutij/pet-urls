package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidLogin         = errors.New("invalid login")
	ErrInvalidPassword      = errors.New("invalid password")
	ErrLoginNotUnique       = errors.New("login not unique")
	ErrLoginDoesNotExist    = errors.New("login does not exist")
	ErrPasswordDoesNotMatch = errors.New("password does not match")
	ErrInvalidToken         = errors.New("invalid token")
	ErrTokenWasRevoked      = errors.New("token was revoked")
	ErrTokenExpired         = errors.New("token expired")
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

	// config parameters
	refreshTokenTTL time.Duration
	accessTokenTTL  time.Duration
}

type Config struct {
	RefreshTokenTTL time.Duration // default: 7 days
	AccessTokenTTL  time.Duration // default: 15 minutes
}

func New(repo repository, config Config, kf func() []byte) Service {
	a := &auth{
		repo:    repo,
		keyFunc: kf,
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
	if err := a.repo.HealthCheck(ctx); err != nil {
		return fmt.Errorf("auth, repo healthcheck: internal") // don't leak
	}

	return nil
}

func (a *auth) Register(ctx context.Context, login, password string) (RefreshToken, AccessToken, error) {
	if len(login) > 64 {
		return nil, nil, fmt.Errorf("auth, registering: %w", ErrInvalidLogin)
	}
	passwordHash, err := hashPassword(password)
	if err != nil {
		return nil, nil, fmt.Errorf("auth, hashing password: %w", ErrInvalidPassword)
	}
	userID, err := a.repo.SaveUser(ctx, UserInRepo{Login: login, PasswordHash: passwordHash})
	if isNotUniqueError(err) {
		return nil, nil, fmt.Errorf("auth, saving user: %w", ErrLoginNotUnique)
	} else if err != nil {
		return nil, nil, fmt.Errorf("auth, saving user: internal") // don't leak
	}

	refreshToken, err := a.issueRefreshToken(ctx, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("auth, issuing refresh token: %w", err)
	}
	accessToken, err := a.issueAccessToken(userID)
	if err != nil {
		return nil, nil, fmt.Errorf("auth, issuing access token: %w", err)
	}

	return refreshToken, accessToken, nil
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
	user, err := a.repo.GetUser(ctx, login)
	if isNotFoundError(err) {
		return nil, nil, fmt.Errorf("auth, fetching user from repo: %w", ErrLoginDoesNotExist)
	} else if err != nil {
		return nil, nil, fmt.Errorf("auth, fetching user from repo: internal") // don't leak
	}

	if !passwordMatchesHash(password, user.PasswordHash) {
		return nil, nil, fmt.Errorf("auth, validating password: %w", ErrPasswordDoesNotMatch)
	}

	refreshToken, err := a.issueRefreshToken(ctx, user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("auth, issuing refresh token: %w", err)
	}

	accessToken, err := a.issueAccessToken(user.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("auth, issuing access token: %w", err)
	}

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
		ExpiresAt: time.Now().UTC().Add(a.accessTokenTTL), // TODO: check timezone logic in time package and in jwt package
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
	tokenHash := hashToken(tokenCandidate)
	err := a.repo.RevokeRefreshToken(ctx, tokenHash)
	if err != nil {
		return fmt.Errorf("auth, revoking refresh token: internal")
	}

	return nil
}

func (a *auth) Refresh(ctx context.Context, tokenCandidate []byte) (AccessToken, error) {
	tokenHash := hashToken(tokenCandidate)

	refreshToken, err := a.repo.GetRefreshToken(ctx, tokenHash)
	if isNotFoundError(err) {
		return nil, fmt.Errorf("auth, fetching refresh token: %w", ErrInvalidToken)
	} else if err != nil {
		return nil, fmt.Errorf("auth, fetching refresh token: internal") // don't leak
	}

	if refreshToken.RevokedAt != nil {
		return nil, fmt.Errorf("auth, refreshing token: %w", ErrTokenWasRevoked)
	}

	if refreshToken.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("auth, refreshing token: %w", ErrTokenExpired)
	}

	accessToken, err := a.issueAccessToken(refreshToken.UserID)
	if err != nil {
		return nil, fmt.Errorf("auth, issuing access token: %w", err)
	}

	return accessToken, nil
}

func (a *auth) Authenticate(ctx context.Context, tokenCandidate []byte) (UserID, error) {
	userID, err := parseJWT(string(tokenCandidate), a.keyFunc) // might return ErrTokenExpired
	if errors.Is(err, ErrTokenExpired) {
		return nil, fmt.Errorf("auth, validating access token: %w", err)
	} else if err != nil {
		return nil, fmt.Errorf("auth, validating access token: internal") // don't leak
	}

	return userID, nil
}

func (a *auth) RefreshTokenTTL() time.Duration { return a.refreshTokenTTL }
