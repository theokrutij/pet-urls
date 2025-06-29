package shortener

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type mockRepo struct {
	token URLToken
}

func (m *mockRepo) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockRepo) SaveToken(cxt context.Context, token URLToken) error {
	m.token = token
	return nil
}

func (m *mockRepo) GetToken(ctx context.Context, tokenStr string) (URLToken, error) {
	return m.token, nil
}

func (m *mockRepo) DeleteToken(ctx context.Context, tokenStr string) error {
	return nil
}

type mockCache struct {
	token URLToken
}

func (m *mockCache) HealthCheck(ctx context.Context) error {
	return nil
}

func (m *mockCache) SaveToken(ctx context.Context, token URLToken, ttl time.Duration) error {
	m.token = token
	return nil
}

func (m *mockCache) GetToken(ctx context.Context, tokenStr string) (URLToken, error) {
	return m.token, nil
}

func (m *mockCache) DeleteToken(ctx context.Context, tokenStr string) error {
	return nil
}

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
			input: CreateTokenInput{URL: "http://o" + strings.Repeat("k.", 996)},
			wantToken: func(t URLToken) bool {
				return string(t.URL) == "http://o"+strings.Repeat("k.", 996)
			},
		},
		{
			name:    "too long url",
			input:   CreateTokenInput{URL: "http://ok" + strings.Repeat("k.", 996)},
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
		{
			name:    "too long token",
			input:   CreateTokenInput{URL: "http://ok", Token: strings.Repeat("a", 65)},
			wantErr: ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &shortener{
				tokenTTL: time.Minute,
				cacheTTL: time.Second * 5,
				repo:     &mockRepo{},
				cache:    &mockCache{},
			}
			got, err := s.createToken(context.Background(), tt.input)
			if tt.wantErr != nil {
				if err == nil || !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.wantToken != nil {
				return
			}

			if !tt.wantToken(got) {
				t.Fatalf("unexpected token returned: %.100s", got)
			}

			if tokenInRepo, _ := s.repo.GetToken(context.Background(), ""); !tt.wantToken(tokenInRepo) || got.Token != tokenInRepo.Token {
				t.Fatalf("unexpected token passed to repo: %.100s", got)
			}

			if tokenInCache, _ := s.cache.GetToken(context.Background(), ""); !tt.wantToken(tokenInCache) || got.Token != tokenInCache.Token {
				t.Fatalf("unexpected token passed to cache: %.100s", got)
			}
		})
	}
}
