package shortener

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/btcsuite/btcutil/base58"
)

// Sentinel errors
var (
	ErrInvalidURL        = errors.New("invalid URL")
	ErrTokenExpired      = errors.New("token expired")
	ErrTokenDoesNotExist = errors.New("token does not exist")
	ErrTokenIsNotUnique  = errors.New("token is not unique")
	ErrInvalidToken      = errors.New("invalid token")
)

type Token string
type URL string

type URLToken struct {
	Token     Token
	URL       URL
	ExpiresAt *time.Time
	ownerID   []byte
}

type CreateTokenInput struct {
	URL     string
	TTL     time.Duration
	Token   string
	OwnerID []byte
}

type Config struct {
	CacheTTL           time.Duration // default: 10 minutes
	TokenTTL           time.Duration // default: 1 hour
	TokenDecodedLength int           // default: 8
}

type shortener struct {
	repo  repository
	cache cache

	// config parameters
	cacheTTL           time.Duration
	tokenTTL           time.Duration
	tokenDecodedLength int
}

func New(repo repository, cache cache, config Config) Service {
	s := &shortener{repo: repo, cache: cache}
	s = applyConfig(s, config)
	return s
}

func applyConfig(s *shortener, c Config) *shortener {
	if c.CacheTTL == 0 {
		s.cacheTTL = 10 * time.Minute
	} else {
		s.cacheTTL = c.CacheTTL
	}

	if c.TokenTTL == 0 {
		s.tokenTTL = 1 * time.Hour
	} else {
		s.tokenTTL = c.TokenTTL
	}

	if c.TokenDecodedLength == 0 {
		s.tokenDecodedLength = 8
	} else {
		s.tokenDecodedLength = c.TokenDecodedLength
	}

	return s
}

// TODO: add cache healthcheck
func (s *shortener) HealthCheck(ctx context.Context) error {
	if err := s.repo.HealthCheck(ctx); err != nil {
		return fmt.Errorf("shortener, checking health: %w", err)
	}
	return nil
}

func (s *shortener) GenerateToken(ctx context.Context, rawURL string) (URLToken, error) {
	createTokenInput := CreateTokenInput{URL: rawURL}
	return s.createToken(ctx, createTokenInput)
}

func (s *shortener) CreateTokenWithOwner(ctx context.Context, input CreateTokenInput) (URLToken, error) {
	var output URLToken
	if input.OwnerID == nil {
		return output, errors.New("shortener, creating token with owner: owner cannot be empty")
	}

	token, err := s.createToken(ctx, input)
	if errors.Is(err, ErrTokenIsNotUnique) {
		return output, fmt.Errorf("shortener, creating token with owner: %w", err)
	}

	return token, err
}

func (s *shortener) createToken(ctx context.Context, input CreateTokenInput) (URLToken, error) {
	var output URLToken

	httpURL, err := normalizeHTTP(string(input.URL))
	if err != nil {
		return output, ErrInvalidURL
	}
	output.URL = httpURL

	if utf8.RuneCountInString(input.Token) > 64 {
		return output, ErrInvalidToken
	}

	var ttl time.Duration
	if input.TTL > 0 {
		ttl = input.TTL
	} else {
		ttl = s.tokenTTL
	}
	exp := time.Now().UTC().Add(ttl)
	output.ExpiresAt = &exp

	if input.Token != "" {
		output.Token = Token(input.Token)
	} else {
		output.Token, err = s.generateRandomBase58()
		if err != nil {
			return output, fmt.Errorf("generating random token: %w", err)
		}
	}

	// Save to database
	// NOTE: token collision treated as critical error, p ≈ 5.42e-20
	err = s.repo.SaveToken(ctx, output)
	if isNotUniqueError(err) {
		return output, fmt.Errorf("saving to repo: %w", ErrTokenIsNotUnique)
	} else if err != nil {
		return output, fmt.Errorf("saving to repo: %w", err)
	}

	// Storing new token in cache, ignoring errors for now
	// TODO: log cache failure
	_ = s.cache.SaveToken(ctx, output, s.cacheTTL)

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

type notUniqueError interface {
	NotUnique() bool
}

func isNotUniqueError(err error) bool {
	var nuErr notUniqueError
	return errors.As(err, &nuErr) && nuErr.NotUnique()
}

func (s *shortener) ResolveToken(ctx context.Context, tokenStr string) (URL, error) {
	// Check the cache first
	// TODO: log cache failure
	if token, err := s.cache.GetToken(ctx, tokenStr); err == nil {
		if token.ExpiresAt.Before(time.Now()) {
			return "", fmt.Errorf("shortener, fetching token from cache: %w, token=%s", ErrTokenExpired, token.Token)
		}
		return token.URL, nil
	}

	// If not found in cache, retrieve from the database
	token, err := s.repo.GetToken(ctx, tokenStr)
	if isNotFoundError(err) {
		return "", fmt.Errorf("shortener, fetching token from db: %w | token=%s", ErrTokenDoesNotExist, token.Token)
	} else if err != nil {
		return "", fmt.Errorf("shortener, fetching token from db: %w | token=%s", err, token.Token)
	}

	if token.ExpiresAt.Before(time.Now()) {
		return "", fmt.Errorf("shortener.GetOriginalURL: %w", ErrTokenExpired)
	}

	// Update cache
	// TODO: handle cache failure
	s.cache.SaveToken(ctx, token, s.cacheTTL)

	return token.URL, nil
}

type NotFoundError interface {
	NotFound() bool
}

func isNotFoundError(err error) bool {
	var nfErr NotFoundError
	return errors.As(err, &nfErr) && nfErr.NotFound()
}
