package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

// TestShortenerHealthCheck verifies that shortener.HealthCheck returns no error
// when all its dependencies are healthy and accessible.
func TestShortenerHealthcheck(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	repo := postgres.NewShortenerRepository(pg)
	cache, _ := setupTestCache(testCtx, t)
	testShortener := shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{})

	err := testShortener.HealthCheck(testCtx)

	assert.NoError(t, err)
}

// TestGenerateToken verifies that the GenerateToken method correctly creates a token,
// writes it to the database, and returns the expected data.
func TestGenerateToken(t *testing.T) {
	// setup
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	repo := postgres.NewShortenerRepository(pg)
	cache, redisClient := setupTestCache(testCtx, t)
	tokenTTL := time.Hour
	testShortener := shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{TokenTTL: tokenTTL})

	// execution
	tokenOutput, err := testShortener.GenerateToken(testCtx, "http://example.com")
	assert.NoError(t, err)

	// assertions
	assert.EqualValues(t, "http://example.com", tokenOutput.URL)

	// DB
	var tokenInDB shortener.URLToken
	q := `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	err = pg.QueryRow(testCtx, q, tokenOutput.Token).Scan(&tokenInDB.URL, &tokenInDB.ExpiresAt, &tokenInDB.OwnerID)
	assert.NoError(t, err)
	assert.Equal(t, tokenOutput.URL, tokenInDB.URL)
	assert.WithinDuration(t, time.Now().Add(tokenTTL), tokenInDB.ExpiresAt, acceptableTimePrecision)
	assert.Nil(t, tokenInDB.OwnerID)

	// Cache
	v, err := redisClient.Get(testCtx, string(tokenOutput.Token)).Result()
	assert.NoError(t, err)
	assert.EqualValues(t, tokenOutput.URL, v)
}

// TestResolveToken verifies that the ResolveToken method correctly fetches URL
// from a database row that has a matching value in the token column.
func TestResolveTokenNoCache(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	q := `
		INSERT INTO url_tokens (token, url, valid_until, owner_id)
		VALUES ($1, $2, $3, $4)
	`
	tokenInDB := shortener.URLToken{
		Token:     "testtoken",
		URL:       "http://example.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		OwnerID:   nil,
	}
	_, err := pg.Exec(testCtx, q, tokenInDB.Token, tokenInDB.URL, tokenInDB.ExpiresAt, tokenInDB.OwnerID)
	assert.NoError(t, err)

	repo := postgres.NewShortenerRepository(pg)
	cache, _ := setupTestCache(testCtx, t)
	testShortener := shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{})

	outputURL, err := testShortener.ResolveToken(testCtx, string(tokenInDB.Token))
	assert.NoError(t, err)

	assert.EqualValues(t, tokenInDB.URL, outputURL)
}

func TestResolveTokenCacheHit(t *testing.T) {
	// setup
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	cache, redisClient := setupTestCache(testCtx, t)

	err := redisClient.Set(testCtx, "testToken", "http://example.com", 10*time.Minute).Err()
	assert.NoError(t, err)

	pg := setupTestPostgres(testCtx, t)
	repo := postgres.NewShortenerRepository(pg)
	testShortener := shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{})

	// execution
	gotURL, gotErr := testShortener.ResolveToken(testCtx, "testToken")

	// assertions
	assert.NoError(t, gotErr)
	assert.EqualValues(t, "http://example.com", gotURL)

}

// TestCreateTokenWithOwner verifies that CreateTokenWithOwner method
// correcty creates a token with provided URL, TTL, Token and OwnerID fields,
// writes it to the database and returns the expected data.
// It checks that:
// - No error is returned from CreateTokenWithOwner
// - The returned token matches every field of the input token
// - The token saved in the database matches every field of the input token
// - The token exists in cache as a key with correct URL as value
func TestCreateTokenWithOwner(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	repo := postgres.NewShortenerRepository(pg)
	cache, redisClient := setupTestCache(testCtx, t)
	testShortener := shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{})

	tokenInput := shortener.CreateTokenInput{
		URL:     "http://example.com",
		TTL:     time.Hour,
		Token:   "abcd",
		OwnerID: []byte("testuser"),
	}

	tokenOutput, err := testShortener.CreateTokenWithOwner(testCtx, tokenInput)

	// assertions
	assert.NoError(t, err)

	assert.EqualValues(t, tokenInput.URL, tokenOutput.URL)
	assert.WithinDuration(t, time.Now().Add(tokenInput.TTL), tokenOutput.ExpiresAt, acceptableTimePrecision)
	assert.EqualValues(t, tokenInput.Token, tokenOutput.Token)
	assert.Equal(t, tokenInput.OwnerID, tokenOutput.OwnerID)

	// DB assertions
	var tokenInDB shortener.URLToken
	q := `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	err = pg.QueryRow(testCtx, q, tokenOutput.Token).Scan(&tokenInDB.URL, &tokenInDB.ExpiresAt, &tokenInDB.OwnerID)
	assert.NoError(t, err)
	assert.EqualValues(t, tokenInput.URL, tokenInDB.URL)
	assert.WithinDuration(t, time.Now().Add(tokenInput.TTL), tokenInDB.ExpiresAt, acceptableTimePrecision)
	assert.Equal(t, tokenInput.OwnerID, tokenInDB.OwnerID)

	// Cache assertions
	v, err := redisClient.Get(testCtx, string(tokenOutput.Token)).Result()
	assert.NoError(t, err)
	assert.EqualValues(t, tokenOutput.URL, v)
}

// TestDeleteToken verifies that DeleteToken method
// correctly deletes the token when token.Token is matched by tokenStr argument
// and token.OwnerID is matched by requestingUserID argument
// It checks that:
//   - No error is returned from DeleteToken
//   - After DeleteToken successfully returns, the token no longer exists in the database or cache
func TestDeleteToken(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	q := `
		INSERT INTO url_tokens (token, url, valid_until, owner_id)
		VALUES ($1, $2, $3, $4)
	`
	targetToken := shortener.URLToken{
		Token:     "testtoken",
		URL:       "http://example.com",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		OwnerID:   nil,
	}
	_, err := pg.Exec(testCtx, q, targetToken.Token, targetToken.URL, targetToken.ExpiresAt, targetToken.OwnerID)
	assert.NoError(t, err)

	cache, redisClient := setupTestCache(testCtx, t)
	err = redisClient.Set(testCtx, string(targetToken.Token), string(targetToken.URL), time.Hour).Err()
	assert.NoError(t, err)

	repo := postgres.NewShortenerRepository(pg)
	testShortener := shortener.New(shortener.Dependencies{Repo: repo, Cache: cache}, shortener.Config{})

	// execution
	err = testShortener.DeleteToken(testCtx, string(targetToken.Token), targetToken.OwnerID)
	assert.NoError(t, err)

	// DB assertions
	q = `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	err = pg.QueryRow(testCtx, q, targetToken.Token).Scan()
	assert.ErrorIs(t, err, pgx.ErrNoRows)

	// Cache assertions
	_, err = redisClient.Get(testCtx, string(targetToken.Token)).Result()
	assert.ErrorIs(t, err, redis.Nil)
}
