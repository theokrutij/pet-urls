package shortener

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcutil/base58"
	"github.com/rs/zerolog"

	xcontext "github.com/theokrutij/pet-urls/internal/context"
)

// ------- Constants and sentinel errors -------

const (
	defaultTokenTTL    = 1 * time.Hour
	defaultCacheTTL    = 10 * time.Minute
	defaultTokenLength = 8
)

var (
	ErrInvalidURL        = errors.New("invalid URL")
	ErrEmptyOwner        = errors.New("empty owner")
	ErrTokenExpired      = errors.New("token expired")
	ErrTokenDoesNotExist = errors.New("token does not exist")
	ErrTokenIsNotUnique  = errors.New("token is not unique")
	ErrInvalidToken      = errors.New("invalid token")
	ErrNotTokenOwner     = errors.New("requesting user is not the token owner")
)

// ------- Types -------

type Token string
type URL string

type ID []byte

func (id ID) String() string {
	return base64.RawURLEncoding.EncodeToString(id)
}

type URLToken struct {
	Token     Token
	URL       URL
	ExpiresAt *time.Time
	OwnerID   ID
}

type CreateTokenInput struct {
	URL     string
	TTL     time.Duration
	Token   string
	OwnerID ID
}

type Dependencies struct {
	Repo  repository
	Cache cache
}

type Config struct {
	CacheTTL           time.Duration  // default: 10 minutes
	TokenTTL           time.Duration  // default: 1 hour
	TokenDecodedLength int            // default: 8
	Logger             zerolog.Logger // default: no logging
}

type shortener struct {
	// dependencies
	repo  repository
	cache cache

	// config parameters
	cacheTTL           time.Duration
	tokenTTL           time.Duration
	tokenDecodedLength int
	logger             zerolog.Logger
}

// ------- Constructor --------

func New(deps Dependencies, config Config) Service {
	s := &shortener{repo: deps.Repo, cache: deps.Cache}
	s = applyConfig(s, config)
	return s
}

func applyConfig(s *shortener, c Config) *shortener {
	if c.CacheTTL == 0 {
		s.cacheTTL = defaultCacheTTL
	} else {
		s.cacheTTL = c.CacheTTL
	}

	if c.TokenTTL == 0 {
		s.tokenTTL = defaultTokenTTL
	} else {
		s.tokenTTL = c.TokenTTL
	}

	if c.TokenDecodedLength == 0 {
		s.tokenDecodedLength = defaultTokenLength
	} else {
		s.tokenDecodedLength = c.TokenDecodedLength
	}

	s.logger = c.Logger

	return s
}

// ------- Public methods -------

// HealthCheck implements Service.HealthCheck.
func (s *shortener) HealthCheck(ctx context.Context) error {
	logger := s.loggerWithRequestID(ctx)
	if err := s.repo.HealthCheck(ctx); err != nil {
		logger.Error().
			Err(err).
			Msg("repo healthcheck fail")
		return fmt.Errorf("shortener, repo healthcheck: %w", err)
	}
	if err := s.cache.HealthCheck(ctx); err != nil {
		logger.Warn().
			Err(err).
			Msg("cache healthcheck fail")
	}

	return nil
}

// GenerateToken implements Service.GenerateToken
func (s *shortener) GenerateToken(ctx context.Context, rawURL string) (URLToken, error) {
	logger := s.loggerWithRequestID(ctx)
	createTokenInput := CreateTokenInput{URL: rawURL}
	output, err := s.createToken(ctx, createTokenInput)
	if err == nil {
		logger.Info().
			Str("token", string(output.Token)).
			Str("url", string(output.URL)).
			Time("expires_at", *output.ExpiresAt).
			Msg("GenerateToken: success")
	}

	return output, err
}

// ResolveToken implements Service.ResolveToken
func (s *shortener) ResolveToken(ctx context.Context, tokenStr string) (URL, error) {
	logger := s.loggerWithRequestID(ctx)

	// Check the cache first
	token, ok, cacheErr := s.cache.GetToken(ctx, tokenStr)
	if cacheErr != nil {
		logger.Warn().
			Err(cacheErr).
			Msg("ResolveToken: failed to fetch token from cache")
	} else if ok {
		if token.ExpiresAt.Before(time.Now()) {
			logger.Info().
				Str("token", string(token.Token)).
				Time("exp", *token.ExpiresAt).
				Msg("Requested token expired")
			return "", fmt.Errorf("shortener, fetching token from cache: %w, token=%s", ErrTokenExpired, token.Token)
		}
		return token.URL, nil
	}

	// If not found in cache, retrieve from the database
	token, repoErr := s.repo.GetToken(ctx, tokenStr)
	if isNotFoundError(repoErr) {
		logger.Info().
			Str("token", tokenStr).
			Msg("ResolveToken: not found in repo")
		return "", fmt.Errorf("shortener, fetching token from db: %w | token=%s", ErrTokenDoesNotExist, token.Token)
	} else if repoErr != nil {
		logger.Error().
			Err(repoErr).
			Msg("ResolveToken: failed to fetch token from repository")
		return "", fmt.Errorf("shortener, fetching token from db: %w | token=%s", repoErr, token.Token)
	}

	if token.ExpiresAt.Before(time.Now()) {
		logger.Info().
			Str("token", string(token.Token)).
			Time("exp", *token.ExpiresAt).
			Msg("Requested token expired")
		return "", fmt.Errorf("shortener.GetOriginalURL: %w", ErrTokenExpired)
	}

	// Update cache
	cacheErr = s.cache.SaveToken(ctx, token, s.cacheTTL)
	if cacheErr != nil {
		logger.Warn().
			Err(cacheErr).
			Msg("ResolveToken: failed to save token to cache")
	}

	logEvent := logger.Info().
		Str("token", tokenStr).
		Str("url", string(token.URL))
	if token.OwnerID != nil {
		logEvent.Str("owner_id", token.OwnerID.String())
	}
	logEvent.Msg("ResolveToken: success")

	return token.URL, nil
}

// CreateTokenWithOwner implements Service.CreateTokenWithOwner
func (s *shortener) CreateTokenWithOwner(ctx context.Context, input CreateTokenInput) (URLToken, error) {
	logger := s.loggerWithRequestID(ctx)

	if input.OwnerID == nil {
		logger.Error().
			Msg("CreateTokenWithOwner: must receive non-nil owner")
		return URLToken{}, fmt.Errorf("shortener, creating token with owner: %w", ErrEmptyOwner)
	}

	output, err := s.createToken(ctx, input)
	if errors.Is(err, ErrTokenIsNotUnique) {
		logger.Info().
			Str("token", input.Token).
			Msg("CreateTokenWithOwner: token not unique")
		return output, fmt.Errorf("shortener, creating token with owner: %w", err)
	}

	if err == nil {
		logger.Info().
			Str("token", string(output.Token)).
			Str("url", string(output.URL)).
			Str("owner_id", string(output.OwnerID)).
			Time("expires_at", *output.ExpiresAt).
			Msg("CreateTokenWithOwner: success")
	}

	return output, err
}

// DeleteToken implements Service.DeleteTokenWithOwner
func (s *shortener) DeleteToken(ctx context.Context, tokenStr string, requestingUserID []byte) error {
	logger := s.loggerWithRequestID(ctx)

	token, err := s.repo.GetToken(ctx, tokenStr)
	if isNotFoundError(err) {
		logger.Info().
			Str("token", tokenStr).
			Msg("DeleteToken: token doesn't exist")
		return nil
	} else if err != nil {
		logger.Error().
			Str("token", tokenStr).
			Err(err).
			Msg("DeleteToken: failed to fetch token from repo")
		return fmt.Errorf("shortener, fetching token from repo for deletion: %w", err)
	}
	if !bytes.Equal(token.OwnerID, requestingUserID) {
		logger.Warn().
			Str("token", tokenStr).
			Str("requesting_user_id", string(requestingUserID)).
			Str("owner_id", string(token.OwnerID)).
			Msg("DeleteToken: non-onwer attempted deleting token")
		return fmt.Errorf("shortener, deleting token: %w", ErrNotTokenOwner)
	}

	// TODO: handle cache failure
	cacheErr := s.cache.DeleteToken(ctx, tokenStr)
	if cacheErr != nil {
		logger.Error().
			Err(cacheErr).
			Msg("DeleteToken: failed to delete token from cache")
		return cacheErr
	}

	repoErr := s.repo.DeleteToken(ctx, tokenStr)
	if isNotFoundError(repoErr) {
		logger.Info().
			Str("token", tokenStr).
			Str("user_id", token.OwnerID.String()).
			Msg("DeleteToken: token doesn't exist, returning ok")
		return nil
	} else if repoErr != nil {
		logger.Error().
			Err(repoErr).
			Msg("DeleteToken: failed to delete token from repo")
		return fmt.Errorf("shortener, deleting token: %w", repoErr)
	}

	logger.Info().
		Str("token", tokenStr).
		Str("user_id", token.OwnerID.String()).
		Msg("DeleteToken: success")
	return nil
}

// ------- Internal helper methods -------

// createToken validates input fields, sets default values if needed, and
// persists token in repo and cache.
//
// Validation rules:
//   - input.URL must be a valid HTTP URL (missing scheme is interpreted as http://).
//   - input.Token must not contain more than 64 runes
//
// Default values:
//   - If input.Token is an empty string, a random base58 encoded byte sequence is used as token.
//   - If input.TTL is zero or negative, a default TTL is set
func (s *shortener) createToken(ctx context.Context, input CreateTokenInput) (URLToken, error) {
	logger := s.loggerWithRequestID(ctx)
	logger.Debug().
		Str("url", input.URL).
		Dur("ttl", input.TTL).
		Str("token", input.Token).
		Msg("createToken: starting")
	var output URLToken

	// URL
	httpURL, err := normalizeHTTP(string(input.URL))
	if err != nil {
		logger.Warn().
			Str("url", input.URL).
			Err(err).
			Msg("createToken: invalid url")
		return output, ErrInvalidURL
	}
	output.URL = httpURL

	// TTL
	var ttl time.Duration
	if input.TTL > 0 {
		ttl = input.TTL
	} else {
		logger.Debug().
			Dur("default_TTL", s.tokenTTL).
			Msg("createToken: setting default token TTL")
		ttl = s.tokenTTL
	}
	exp := time.Now().UTC().Add(ttl)
	output.ExpiresAt = &exp

	// Token
	if utf8.RuneCountInString(input.Token) > 64 {
		logger.Warn().
			Str("token", input.Token).
			Msg("createToken: token too long")
		return output, ErrInvalidToken
	}
	if input.Token != "" {
		output.Token = Token(input.Token)
	} else {
		output.Token, err = s.generateRandomBase58()
		logger.Debug().
			Str("token", string(output.Token)).
			Msg("createToken: setting random base58 token")
		if err != nil {
			return output, fmt.Errorf("generating random token: %w", err)
		}
	}

	// OwnerID
	output.OwnerID = input.OwnerID

	// Save to database
	err = s.repo.SaveToken(ctx, output)
	if isNotUniqueError(err) {
		logger.Debug().
			Str("token", string(output.Token)).
			Msg("createToken: token not unique")
		return output, fmt.Errorf("saving to repo: %w", ErrTokenIsNotUnique)
	} else if err != nil {
		logger.Error().
			Str("token", string(output.Token)).
			Err(err).
			Msg("createToken: failed to save token to repo")
		return output, fmt.Errorf("saving to repo: %w", err)
	}

	// Save to cache
	err = s.cache.SaveToken(ctx, output, s.cacheTTL)
	if err != nil {
		logger.Warn().
			Str("token", string(output.Token)).
			Err(err).
			Msg("createToken: failed to save token to cache")
	}
	return output, nil
}

func (s *shortener) generateRandomBase58() (Token, error) {
	raw := make([]byte, s.tokenDecodedLength)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tokenStr := base58.Encode(raw)
	return Token(tokenStr), nil
}

// ------- Error type checkers -------

func isNotUniqueError(err error) bool {
	type notUniqueError interface {
		NotUnique() bool
	}

	var nuErr notUniqueError
	return errors.As(err, &nuErr) && nuErr.NotUnique()
}

func isNotFoundError(err error) bool {
	type notFoundError interface {
		NotFound() bool
	}

	var nfErr notFoundError
	return errors.As(err, &nfErr) && nfErr.NotFound()
}

// ------- Context utils -------

func (s *shortener) loggerWithRequestID(ctx context.Context) zerolog.Logger {
	requestID, ok := xcontext.RequestID(ctx)
	if !ok {
		s.logger.Warn().Msg("no requestID in context")
		return s.logger
	}

	return s.logger.With().Str("request_id", requestID).Logger()
}
