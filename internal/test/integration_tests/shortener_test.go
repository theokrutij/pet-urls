package integration_tests

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

const (
	testTimeout             = time.Second
	acceptableTimePrecision = time.Second
)

// TestHealthCheck verifies that shortener.HealthCheck returns no error
// when all its dependencies are healthy and accessible.
func TestHealthcheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tx := beginTx(ctx, t)
	testShortener := setupTestShortener(t, tx)

	err := testShortener.HealthCheck(ctx)

	if err != nil {
		t.Fatalf("got error from healthcheck: %v", err)
	}
}

// TestGenerateToken verifies that the GenerateToken method correctly creates a token,
// writes it to the database, and returns the expected data.
// It checks that:
// - No error is returned from GenerateToken
// - The returned token's URL matches the input URL
// - The token saved in the database matches the returned token's URL and expiry
// - The OwnerID field in the database is nil as expected
func TestGenerateToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tx := beginTx(ctx, t)
	testShortener := setupTestShortener(t, tx)

	tokenOutput, err := testShortener.GenerateToken(ctx, "http://example.com")
	if err != nil {
		t.Fatalf("got error from GenerateToken: %v", err)
	}

	if tokenOutput.URL != "http://example.com" {
		t.Fatalf("got unexpected URL: %v", tokenOutput.URL)
	}

	var tokenInDB shortener.URLToken
	q := `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	err = tx.QueryRow(ctx, q, tokenOutput.Token).Scan(&tokenInDB.URL, &tokenInDB.ExpiresAt, &tokenInDB.OwnerID)
	if err != nil {
		t.Fatalf("couldn't fetch token from postgres: %v", err)
	}

	if tokenOutput.URL != tokenInDB.URL {
		t.Fatalf("expected URL: %v in DB, got: %v", tokenOutput.URL, tokenInDB.URL)
	}
	if tokenOutput.ExpiresAt.Equal(*tokenInDB.ExpiresAt) {
		t.Fatalf("expected ExpiresAt: %+v in DB, got: %+v", tokenOutput.ExpiresAt, tokenInDB.ExpiresAt)
	}
	if tokenInDB.OwnerID != nil {
		t.Fatalf("expected nil owner in DB, got: %v", tokenInDB.OwnerID)
	}
}

// TestResolveToken verifies that the ResolveToken correctly fetches URL
// from a database row that has a matching value in the token column.
func TestResolveToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tx := beginTx(ctx, t)
	q := `
		INSERT INTO url_tokens (token, url, valid_until, owner_id)
		VALUES ($1, $2, $3, $4)
	`
	exp := time.Now().UTC().Add(time.Hour)
	tokenInDB := shortener.URLToken{
		Token:     "testtoken",
		URL:       "http://example.com",
		ExpiresAt: &exp,
		OwnerID:   nil,
	}
	_, err := tx.Exec(ctx, q, tokenInDB.Token, tokenInDB.URL, tokenInDB.ExpiresAt, tokenInDB.OwnerID)
	if err != nil {
		t.Fatalf("couldn't insert token in postgres: %v", err)
	}

	testShortener := setupTestShortener(t, tx)

	outputURL, err := testShortener.ResolveToken(ctx, string(tokenInDB.Token))
	if err != nil {
		t.Fatalf("got error from ResolveToken: %v", err)
	}

	if outputURL != tokenInDB.URL {
		t.Fatalf("expected url from ResolveToken: %s, got: %s", tokenInDB.URL, outputURL)
	}
}

// TestCreateTokenWithOwner verifies that CreateTokenWithOwner method
// correcty creates a token with provided URL, TTL, Token and OwnerID fields,
// writes it to the database and returns the expected data.
// It checks that:
// - No error is returned from CreateTokenWithOwner
// - The returned token matches every field of the input token
// - The token saved in the database matches every field of the input token
func TestCreateTokenWithOwner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tx := beginTx(ctx, t)
	testShortener := setupTestShortener(t, tx)

	tokenInput := shortener.CreateTokenInput{
		URL:     "http://example.com",
		TTL:     time.Hour,
		Token:   "abcd",
		OwnerID: []byte("testuser"),
	}

	tokenOutput, err := testShortener.CreateTokenWithOwner(ctx, tokenInput)
	if err != nil {
		t.Fatalf("got error from CreateTokenWithOwner: %v", err)
	}

	if tokenOutput.URL != shortener.URL(tokenInput.URL) {
		t.Fatalf("expected URL in output: %s, got: %s", tokenInput.URL, tokenOutput.URL)
	}
	outputTTL := time.Until(*tokenOutput.ExpiresAt)
	if time.Duration.Abs(outputTTL-tokenInput.TTL) > acceptableTimePrecision {
		t.Fatalf("expected TTL in output: %s, got %s", tokenInput.TTL, outputTTL)
	}
	if string(tokenOutput.Token) != tokenInput.Token {
		t.Fatalf("expected Token in output: %s, got: %s", tokenInput.Token, tokenOutput.Token)
	}
	if !bytes.Equal(tokenOutput.OwnerID, tokenInput.OwnerID) {
		t.Fatalf("expected OwnerID in output: %s, got %s", tokenInput.OwnerID, tokenOutput.OwnerID)
	}

	var tokenInDB shortener.URLToken
	q := `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	err = tx.QueryRow(ctx, q, tokenOutput.Token).Scan(&tokenInDB.URL, &tokenInDB.ExpiresAt, &tokenInDB.OwnerID)
	if err != nil {
		t.Fatalf("couldn't fetch token from postgres: %v", err)
	}

	if string(tokenInDB.URL) != tokenInput.URL {
		t.Fatalf("expected URL: %s in DB, got: %s", tokenOutput.URL, tokenInDB.URL)
	}
	TTLinDB := time.Until(*tokenInDB.ExpiresAt)
	if time.Duration.Abs(TTLinDB-tokenInput.TTL) > acceptableTimePrecision {
		t.Fatalf("expected TTL in DB: %v, got: %v", tokenInput.TTL, TTLinDB)
	}
	if !bytes.Equal(tokenInDB.OwnerID, tokenInput.OwnerID) {
		t.Fatalf("expected OwnerID in DB: %v, got: %v", tokenOutput.OwnerID, tokenInDB.OwnerID)
	}
}

// TestDeleteToken verifies that DeleteToken method
// correctly deletes the token when token.Token is matched by tokenStr argument
// and token.OwnerID is matched by requestingUserID argument
// It checks that:
//   - No error is returned from DeleteToken
//   - After DeleteToken successfully returns, the token no longer exists in the database
func TestDeleteToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	tx := beginTx(ctx, t)

	q := `
		INSERT INTO url_tokens (token, url, valid_until, owner_id)
		VALUES ($1, $2, $3, $4)
	`

	exp := time.Now().UTC().Add(time.Hour)
	targetToken := shortener.URLToken{
		Token:     "testtoken",
		URL:       "http://example.com",
		ExpiresAt: &exp,
		OwnerID:   nil,
	}
	_, err := tx.Exec(ctx, q, targetToken.Token, targetToken.URL, targetToken.ExpiresAt, targetToken.OwnerID)
	if err != nil {
		t.Fatalf("couldn't insert token in postgres: %v", err)
	}

	testShortener := setupTestShortener(t, tx)

	err = testShortener.DeleteToken(ctx, string(targetToken.Token), targetToken.OwnerID)
	if err != nil {
		t.Fatalf("got error from DeleteToken: %v", err)
	}

	q = `
		SELECT url, valid_until, owner_id FROM url_tokens
		WHERE token = $1
	`
	rows, err := tx.Query(ctx, q, targetToken.Token)
	if err != nil {
		t.Fatalf("couldn't query database: %v", err)
	}
	if rows.Next() {
		t.Fatalf("expected no matching rows in db")
	}
}
