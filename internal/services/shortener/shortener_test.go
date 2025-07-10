package shortener

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCreateToken(t *testing.T) {
	ttlMatches := func(token URLToken, expectedTTL time.Duration) bool {
		tokenTTL := token.ExpiresAt.Sub(time.Now().UTC())
		return (tokenTTL - expectedTTL) < 100*time.Millisecond
	}

	tests := []struct {
		name      string
		input     CreateTokenInput
		wantErr   error
		wantToken func(URLToken) bool
	}{
		{
			name:  "all fields set",
			input: CreateTokenInput{URL: "http://ok", Token: "abc", TTL: time.Minute, OwnerID: []byte("test_user")},
			wantToken: func(t URLToken) bool {
				check := t.URL == "http://ok"
				check = check && t.Token == "abc"
				check = check && ttlMatches(t, time.Minute)
				check = check && bytes.Equal(t.OwnerID, []byte("test_user"))
				return check
			},
		},
		{
			name:  "default TTL and random token",
			input: CreateTokenInput{URL: "http://ok"},
			wantToken: func(t URLToken) bool {
				check := t.URL == "http://ok"
				check = check && ttlMatches(t, defaultTokenTTL)
				check = check && t.OwnerID == nil
				return check
			},
		},
		{
			name:  "long token",
			input: CreateTokenInput{URL: "http://ok", Token: strings.Repeat("a", 64)},
			wantToken: func(t URLToken) bool {
				check := t.URL == "http://ok"
				check = check && string(t.Token) == strings.Repeat("a", 64)
				check = check && ttlMatches(t, defaultTokenTTL)
				return check
			},
		},
		{
			name:    "too long token",
			input:   CreateTokenInput{URL: "http://ok", Token: strings.Repeat("a", 65)},
			wantErr: ErrInvalidToken,
		},
		{
			name:  "negative TTL",
			input: CreateTokenInput{URL: "http://ok", TTL: -5 * time.Hour},
			wantToken: func(t URLToken) bool {
				check := t.URL == "http://ok"
				check = check && ttlMatches(t, defaultTokenTTL)
				check = check && t.OwnerID == nil
				return check
			},
		},
		{
			name:  "very large TTL",
			input: CreateTokenInput{URL: "http://ok", TTL: 292 * 365 * 24 * time.Hour},
			wantToken: func(t URLToken) bool {
				check := t.URL == "http://ok"
				check = check && ttlMatches(t, 292*365*24*time.Hour)
				check = check && t.OwnerID == nil
				return check
			},
		},
		{
			name:  "long url",
			input: CreateTokenInput{URL: "http://abcde.com/" + strings.Repeat("a", 1983)},
			wantToken: func(t URLToken) bool {
				return string(t.URL) == "http://abcde.com/"+strings.Repeat("a", 1983)
			},
		},
		{
			name:    "too long url",
			input:   CreateTokenInput{URL: "http://abcde.com/" + strings.Repeat("a", 1984)},
			wantErr: ErrInvalidURL,
		},
		{
			name:    "invalid url",
			input:   CreateTokenInput{URL: "ht!tp://bad"},
			wantErr: ErrInvalidURL,
		}, {
			name:    "empty url",
			input:   CreateTokenInput{URL: ""},
			wantErr: ErrInvalidURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockRepoInstance := &mockRepo{}
			mockCacheInstance := &mockCache{}
			s := &shortener{
				tokenTTL: time.Minute,
				cacheTTL: time.Second * 5,
				repo:     mockRepoInstance,
				cache:    mockCacheInstance,
			}

			// Action
			got, err := s.createToken(context.Background(), tt.input)

			// Check 1: if testCase expects sentinel error,
			// assert that correct error value is returned
			// and that nothing is passed to repo or cache.
			if tt.wantErr != nil {
				if err == nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				if mockRepoInstance.token != nil {
					t.Fatalf("expected no token passed to repo, got: %+v", *mockRepoInstance.token)
				}

				if mockCacheInstance.token != nil {
					t.Fatalf("expected no token passed to repo, got: %+v", *mockCacheInstance.token)
				}
				return
			}

			// Check 2: if testCase does not expect an error,
			// assert that no error is returned.
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// If testCase is not expected to return a token,
			// the test case passes.
			if tt.wantToken == nil {
				return
			}

			// Check 3: assert that returned token matches expectations
			// that are defined by the wantToken predicate.
			if !tt.wantToken(got) {
				t.Fatalf("unexpected token returned: %.100s", got)
			}

			// Check 4: assert that a token that was passed to repo and cache
			// matches expectations and that token.Token is the same for returned token
			// and for the token that was passed into repo and cache.
			if !tt.wantToken(*mockRepoInstance.token) || got.Token != mockCacheInstance.token.Token {
				t.Fatalf("unexpected token passed to repo: %.100s", got)
			}
			if !tt.wantToken(*mockCacheInstance.token) || got.Token != mockCacheInstance.token.Token {
				t.Fatalf("unexpected token passed to cache: %.100s", got)
			}
		})
	}
}

// mockRepo implements repo dependency interface.
// It stores a single token in memory.
// SaveToken overwrites the stored token.
// GetToken returns the stored token.
// HealthCheck always returns nil.
// DeleteToken is a no-op.
type mockRepo struct {
	token *URLToken
}

func (m *mockRepo) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockRepo) SaveToken(cxt context.Context, token URLToken) error {
	m.token = &token
	return nil
}

func (m *mockRepo) GetToken(ctx context.Context, tokenStr string) (URLToken, error) {
	return *m.token, nil
}

func (m *mockRepo) DeleteToken(ctx context.Context, tokenStr string) error {
	return nil
}

// mockCache implements cache dependecy interface.
// It stores a single token in memory.
// SaveToken overwrites the stored token.
// GetToken returns the stored token.
// HealthCheck always returns nil.
// DeleteToken is a no-op.
type mockCache struct {
	token *URLToken
}

func (m *mockCache) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockCache) SaveToken(ctx context.Context, token URLToken, ttl time.Duration) error {
	m.token = &token
	return nil
}

func (m *mockCache) GetToken(ctx context.Context, tokenStr string) (URLToken, bool, error) {
	return *m.token, true, nil
}

func (m *mockCache) DeleteToken(ctx context.Context, tokenStr string) error {
	return nil
}
