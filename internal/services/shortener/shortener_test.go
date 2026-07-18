package shortener

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	validURL                = "example.com"
	acceptableTimePrecision = time.Second
)

var (
	errGeneric = errors.New("oops")
)

type dependenciesSetup func() (repository, cache)
type tokenMatcher func(token URLToken) bool

// TestHealthCheck verifies that the HealthCheck method:
//   - Calls repo.HealthCheck and cache.Healthcheck.
//   - Returns nil if both dependencies return nil.
//   - Returns a generic error if repository returns an error.
func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name                string
		depSetup            dependenciesSetup
		cacheShouldBeCalled bool
		wantAnyError        bool
	}{
		{
			name: "healthy",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("HealthCheck", mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("HealthCheck", mock.Anything).Return(nil)

				return repo, cache
			},
			cacheShouldBeCalled: true,
		},
		{
			name: "repo unhealthy",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("HealthCheck", mock.Anything).Return(errGeneric)

				cache := new(mockCache)

				return repo, cache
			},
			wantAnyError: true,
		},
		{
			name: "cache unhealthy",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("HealthCheck", mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("HealthCheck", mock.Anything).Return(errGeneric)
				return repo, cache
			},
			cacheShouldBeCalled: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// setup
			testCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cache := tt.depSetup()

			testShortener := New(
				Dependencies{Repo: repo, Cache: cache},
				Config{},
			)

			// execution
			gotErr := testShortener.HealthCheck(testCtx)

			// assertions
			repo.(*mockRepo).AssertCalled(t, "HealthCheck", testCtx)
			if tt.cacheShouldBeCalled {
				cache.(*mockCache).AssertCalled(t, "HealthCheck", testCtx)
			} else {
				cache.(*mockCache).AssertNumberOfCalls(t, "HealthCheck", 0)
			}

			if tt.wantAnyError {
				assert.Error(t, gotErr)
			} else {
				assert.NoError(t, gotErr)
			}
		})
	}

}

// TestGenerateToken verifies that the GenerateToken method:
//   - Normalizes url or returns ErrInvalidURL if it is not possible.
//   - Generates a random string to be used as a token.
//   - Sets token TTL to default value.
//   - Keeps OwnerID nil.
//   - Calls repo.SaveToken with a correctly set URLToken value.
//   - If saving to repo is successful, calls cache.SaveToken with a correctly set URLToken value and cache value TTL.
//   - Returns a generic error if saving to repo resulted in any error (including token clash).
//   - On success returns the same URLToken that was passed to repo and cache.
func TestGenerateToken(t *testing.T) {
	tests := []struct {
		name                string
		depSetup            dependenciesSetup
		url                 string
		repoShouldBeCalled  bool
		cacheShouldBeCalled bool
		targetURLToken      tokenMatcher
		wantSpecificError   error
		wantGenericError    bool
	}{
		{
			name: "invalid URL",
			depSetup: func() (repository, cache) {
				return new(mockRepo), new(mockCache)
			},
			url:               "ftp://example.com",
			wantSpecificError: ErrInvalidURL,
		},
		{
			name: "valid URL, no errors",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			url:                 "example.com",
			repoShouldBeCalled:  true,
			cacheShouldBeCalled: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					t.OwnerID == nil
			},
		},
		{
			name: "valid URL, not unique error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(mockNotUniqueError{})

				cache := new(mockCache)

				return repo, cache
			},
			url:                "example.com",
			repoShouldBeCalled: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					t.OwnerID == nil
			},
			wantGenericError: true,
		},
		{
			name: "valid URL, generic repo error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(errGeneric)

				cache := new(mockCache)

				return repo, cache
			},
			url:                "example.com",
			repoShouldBeCalled: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					t.OwnerID == nil
			},
			wantGenericError: true,
		},
		{
			name: "valid URL, no repo error, generic cache error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errGeneric)

				return repo, cache
			},
			url:                 "example.com",
			repoShouldBeCalled:  true,
			cacheShouldBeCalled: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					t.OwnerID == nil
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// setup
			testCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cache := tt.depSetup()

			testShortener := New(Dependencies{Repo: repo, Cache: cache}, Config{})

			// execution
			gotToken, gotError := testShortener.GenerateToken(testCtx, tt.url)

			// assertions
			if tt.repoShouldBeCalled {
				repo.(*mockRepo).AssertCalled(
					t,
					"SaveToken",
					testCtx,
					mock.MatchedBy(func(t URLToken) bool {
						if tt.wantSpecificError != nil || tt.wantGenericError {
							return tt.targetURLToken(t)
						} else {
							return gotToken.Token == t.Token && tt.targetURLToken(t)
						}
					}),
				)
			} else {
				repo.(*mockRepo).AssertNumberOfCalls(t, "SaveToken", 0)
			}
			if tt.cacheShouldBeCalled {
				normHTTP, _ := normalizeHTTP(tt.url)
				cache.(*mockCache).AssertCalled(
					t,
					"SaveToken",
					testCtx,
					mock.MatchedBy(func(t Token) bool {
						return gotToken.Token == t
					}),
					normHTTP,
					defaultMaxCacheTTL,
				)
			} else {
				cache.(*mockCache).AssertNumberOfCalls(t, "SaveToken", 0)
			}

			if tt.wantSpecificError != nil {
				assert.ErrorIs(t, gotError, tt.wantSpecificError)
			} else if tt.wantGenericError {
				assert.Error(t, gotError)
				assert.NotErrorIs(t, gotError, ErrInvalidURL)
			} else {
				assert.NoError(t, gotError)

				assert.True(t, tt.targetURLToken(gotToken))
			}

		})
	}

}

// TestResolveToken verifies that the ResolveToken method:
//   - Calls cache.GetToken with a tokenStr parameter.
//   - If cache returns a miss or an error, calls repo.GetToken with tokenStr.
//   - If repo returns notFoundError, returns ErrTokenDoesNotExist.
//   - If repo returns an unexpected error, returns a generic error.
//   - After successfully fetching token from either repo or cache, checks token expiration time.
//   - If token is expired, returns ErrTokenExpired.
//   - If token is active, and cache returned a miss, writes token to cache.
//   - If token is active, returns URLToken and nil error.
func TestResolveToken(t *testing.T) {
	tests := []struct {
		name                    string
		depSetup                dependenciesSetup
		tokenStr                string
		repoShouldBeCalled      bool
		cacheSaveShouldBeCalled bool

		wantURL           string
		wantSpecificError error
		wantAnyError      bool
	}{
		{
			name: "cache hit",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)

				cache := new(mockCache)
				cache.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URL("http://example.com"),
						true,
						nil,
					)

				return repo, cache
			},
			tokenStr: "abcd",
			wantURL:  "http://example.com",
		},
		{
			name: "cache miss",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URLToken{Token: "abcd", URL: "http://example.com", ExpiresAt: time.Now().Add(time.Hour)},
						nil,
					)

				cache := new(mockCache)
				cache.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URL(""),
						false,
						nil,
					)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			tokenStr:                "abcd",
			repoShouldBeCalled:      true,
			cacheSaveShouldBeCalled: true,
			wantURL:                 "http://example.com",
		},
		{
			name: "cache error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URLToken{Token: "abcd", URL: "http://example.com", ExpiresAt: time.Now().Add(time.Hour)},
						nil,
					)

				cache := new(mockCache)
				cache.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URL(""),
						false,
						errGeneric,
					)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errGeneric)

				return repo, cache
			},
			tokenStr:                "abcd",
			repoShouldBeCalled:      true,
			cacheSaveShouldBeCalled: true,
			wantURL:                 "http://example.com",
		},
		{
			name: "cache miss, not found in repo",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URLToken{},
						mockNotFoundError{},
					)

				cache := new(mockCache)
				cache.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URL(""),
						false,
						nil,
					)

				return repo, cache
			},
			tokenStr:           "abcd",
			repoShouldBeCalled: true,
			wantSpecificError:  ErrTokenDoesNotExist,
		},
		{
			name: "cache miss, generic repo error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URLToken{},
						errGeneric,
					)

				cache := new(mockCache)
				cache.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URL(""),
						false,
						nil,
					)

				return repo, cache
			},
			tokenStr:           "abcd",
			repoShouldBeCalled: true,
			wantAnyError:       true,
		},
		{
			name: "cache miss, expired token",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URLToken{Token: "abcd", URL: "http://example.com", ExpiresAt: time.Now().Add(-1 * acceptableTimePrecision)},
						nil,
					)

				cache := new(mockCache)
				cache.
					On("GetToken", mock.Anything, mock.Anything).
					Return(
						URL(""),
						false,
						nil,
					)

				return repo, cache
			},
			tokenStr:           "abcd",
			repoShouldBeCalled: true,
			wantSpecificError:  ErrTokenExpired,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// setup
			testCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cache := tt.depSetup()

			testShortener := New(Dependencies{Repo: repo, Cache: cache}, Config{})

			// execution
			gotURL, gotError := testShortener.ResolveToken(testCtx, tt.tokenStr)

			// assertions
			cache.(*mockCache).AssertCalled(t, "GetToken", testCtx, tt.tokenStr)
			if tt.repoShouldBeCalled {
				repo.(*mockRepo).AssertCalled(t, "GetToken", testCtx, tt.tokenStr)
			} else {
				repo.(*mockRepo).AssertNumberOfCalls(t, "GetToken", 0)
			}
			if tt.cacheSaveShouldBeCalled {
				cache.(*mockCache).AssertCalled(t, "SaveToken", testCtx, Token(tt.tokenStr), gotURL, min(defaultMaxCacheTTL, time.Hour))
			}

			if tt.wantSpecificError != nil {
				assert.ErrorIs(t, gotError, tt.wantSpecificError)
			} else if tt.wantAnyError {
				assert.Error(t, gotError)
				assert.NotErrorIs(t, gotError, ErrTokenDoesNotExist)
				assert.NotErrorIs(t, gotError, ErrTokenExpired)
			} else {
				assert.NoError(t, gotError)

				assert.EqualValues(t, tt.wantURL, gotURL)
			}
		})
	}
}

// TestCreateTokenWithOwner verifies that the CreateTokenWithOwner method:
//   - Normalizes input.URL or returns ErrInvalidURL if it is not possible.
//   - Checks that input.Token contains at most 64 runes, or returns ErrInvalidToken otherwise.
//   - Generates a random string if URLToken is empty.
//   - Sets default token TTL if URLToken.TTL is zero or negative.
//   - Records OwnerID.
//   - Calls repo.SaveToken with normalized url and other correctly set URLToken fields.
//   - If repo returns notUniqueError, returns ErrTokenIsNotUnique
//   - If repo returns an unexpected error, return a generic error.
//   - If saving to repo is successful, calls cache.SaveToken with normalized url and other correctly set URLToken fields.
//   - On success returns the same URLToken that was passed to repo and cache.
func TestCreateTokenWithOwner(t *testing.T) {
	tests := []struct {
		name              string
		input             CreateTokenInput
		depSetup          dependenciesSetup
		shouldCallRepo    bool
		shouldCallCache   bool
		targetURLToken    tokenMatcher
		wantSpecificError error
		wantAnyError      bool
	}{
		{
			name: "empty owner",
			input: CreateTokenInput{
				URL: "example.com",
			},
			depSetup:          func() (repository, cache) { return new(mockRepo), new(mockCache) },
			wantSpecificError: ErrEmptyOwner,
		},
		{
			name: "invalid url",
			input: CreateTokenInput{
				URL:     "ftp://example.com",
				OwnerID: []byte("testuserID"),
			},
			depSetup:          func() (repository, cache) { return new(mockRepo), new(mockCache) },
			wantSpecificError: ErrInvalidURL,
		},
		{
			name: "token too long",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
				Token:   strings.Repeat("x", 65),
			},
			depSetup:          func() (repository, cache) { return new(mockRepo), new(mockCache) },
			wantSpecificError: ErrInvalidToken,
		},
		{
			name: "empty token and TTL",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
			},
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			shouldCallRepo:  true,
			shouldCallCache: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					bytes.Equal(t.OwnerID, []byte("testuserID"))
			},
		},
		{
			name: "negative TTL",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
				TTL:     -1 * time.Hour,
			},
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			shouldCallRepo:  true,
			shouldCallCache: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					bytes.Equal(t.OwnerID, []byte("testuserID"))
			},
		},
		{
			name: "specific token and positive TTL",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
				Token:   "abcd",
				TTL:     13 * time.Hour,
			},
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			shouldCallRepo:  true,
			shouldCallCache: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					13*time.Hour-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					bytes.Equal(t.OwnerID, []byte("testuserID")) &&
					t.Token == "abcd"
			},
		},
		{
			name: "not unique token",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
				Token:   "abcd",
			},
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(mockNotUniqueError{})

				cache := new(mockCache)

				return repo, cache
			},
			shouldCallRepo: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					bytes.Equal(t.OwnerID, []byte("testuserID")) &&
					t.Token == "abcd"
			},
			wantSpecificError: ErrTokenIsNotUnique,
		},
		{
			name: "repo error",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
			},
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(errGeneric)

				cache := new(mockCache)

				return repo, cache
			},
			shouldCallRepo: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					bytes.Equal(t.OwnerID, []byte("testuserID"))
			},
			wantAnyError: true,
		},
		{
			name: "cache error",
			input: CreateTokenInput{
				URL:     "example.com",
				OwnerID: []byte("testuserID"),
			},
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("SaveToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("SaveToken", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(errGeneric)

				return repo, cache
			},
			shouldCallRepo:  true,
			shouldCallCache: true,
			targetURLToken: func(t URLToken) bool {
				return t.URL == "http://example.com" &&
					defaultTokenTTL-time.Until(t.ExpiresAt) < acceptableTimePrecision &&
					bytes.Equal(t.OwnerID, []byte("testuserID"))
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// setup
			testCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cache := tt.depSetup()

			testShortener := New(Dependencies{Repo: repo, Cache: cache}, Config{})

			// execution
			gotToken, gotError := testShortener.CreateTokenWithOwner(testCtx, tt.input)

			// assertions
			if tt.shouldCallRepo {
				repo.(*mockRepo).AssertCalled(
					t,
					"SaveToken",
					testCtx,
					mock.MatchedBy(func(t URLToken) bool {
						if gotError != nil {
							return tt.targetURLToken(t)
						} else {
							return t.Token == gotToken.Token && tt.targetURLToken(t)
						}

					}))
			} else {
				repo.(*mockRepo).AssertNumberOfCalls(t, "SaveToken", 0)
			}
			if tt.shouldCallCache {
				cache.(*mockCache).AssertCalled(
					t,
					"SaveToken",
					testCtx,
					gotToken.Token,
					gotToken.URL,
					defaultMaxCacheTTL,
				)
			} else {
				cache.(*mockCache).AssertNumberOfCalls(t, "SaveToken", 0)
			}

			if tt.wantSpecificError != nil {
				assert.ErrorIs(t, gotError, tt.wantSpecificError)
			} else if tt.wantAnyError {
				assert.Error(t, gotError)
				assert.NotErrorIs(t, gotError, ErrInvalidURL)
				assert.NotErrorIs(t, gotError, ErrInvalidToken)
				assert.NotErrorIs(t, gotError, ErrTokenIsNotUnique)
			} else {
				assert.NoError(t, gotError)

				assert.True(t, tt.targetURLToken(gotToken))
			}
		})
	}
}

// TestDeleteToken verifies that the DeleteToken:
//   - Calls repo.GetToken with tokenStr parameter.
//   - If repo returns notFoundError, returns nil.
//   - If repo.GetToken returns nil error, attempts to match requestingUserID with OwnerID of token in repo.
//     and returns ErrNotTokenOwner if ids don't match.
//   - If ids match, calls repo.DeleteToken with tokenStr parameter, else returns ErrNotTokenOwner.
//   - If repo.DeleteToken returns nil, calls cache.DeleteToken with tokenStr parameter.
//   - If repo.GetToken or repo.DeleteToken return an unexpected error,
//     returns a generic error.
func TestDeleteToken(t *testing.T) {
	tests := []struct {
		name                      string
		depSetup                  dependenciesSetup
		requestingUserID          []byte
		cacheDeleteShouldBeCalled bool
		repoDeleteShouldBeCalled  bool
		wantSpecificError         error
		wantAnyError              bool
	}{
		{
			name: "no errors",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{
						OwnerID: []byte("testuserID"),
					},
					nil,
				)
				repo.On("DeleteToken", mock.Anything, mock.Anything).Return(nil)

				cache := new(mockCache)
				cache.On("DeleteToken", mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			requestingUserID:          []byte("testuserID"),
			cacheDeleteShouldBeCalled: true,
			repoDeleteShouldBeCalled:  true,
		},
		{
			name: "wrong requesting user",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{
						OwnerID: []byte("testuserID"),
					},
					nil,
				)

				cache := new(mockCache)

				return repo, cache
			},
			requestingUserID:  []byte("wronguserID"),
			wantSpecificError: ErrNotTokenOwner,
		},
		{
			name: "repo get token generic error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{},
					errGeneric,
				)

				cache := new(mockCache)

				return repo, cache
			},
			requestingUserID: []byte("wronguserID"),
			wantAnyError:     true,
		},
		{
			name: "cache error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{
						OwnerID: []byte("testuserID"),
					},
					nil,
				)

				cache := new(mockCache)
				cache.On("DeleteToken", mock.Anything, mock.Anything).Return(errGeneric)

				return repo, cache
			},
			requestingUserID:          []byte("testuserID"),
			cacheDeleteShouldBeCalled: true,
			wantAnyError:              true,
		},
		{
			name: "repo get not found error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{},
					mockNotFoundError{},
				)

				cache := new(mockCache)

				return repo, cache
			},
			requestingUserID: []byte("testuserID"),
		},
		{
			name: "repo delete not found error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{
						OwnerID: []byte("testuserID"),
					},
					nil,
				)
				repo.On("DeleteToken", mock.Anything, mock.Anything).Return(mockNotFoundError{})

				cache := new(mockCache)
				cache.On("DeleteToken", mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			requestingUserID:          []byte("testuserID"),
			cacheDeleteShouldBeCalled: true,
			repoDeleteShouldBeCalled:  true,
		},
		{
			name: "repo get generic error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{},
					errGeneric,
				)

				cache := new(mockCache)

				return repo, cache
			},
			requestingUserID: []byte("testuserID"),
			wantAnyError:     true,
		},
		{
			name: "repo delete generic error",
			depSetup: func() (repository, cache) {
				repo := new(mockRepo)
				repo.On("GetToken", mock.Anything, mock.Anything).Return(
					URLToken{
						OwnerID: []byte("testuserID"),
					},
					nil,
				)
				repo.On("DeleteToken", mock.Anything, mock.Anything).Return(errGeneric)

				cache := new(mockCache)
				cache.On("DeleteToken", mock.Anything, mock.Anything).Return(nil)

				return repo, cache
			},
			requestingUserID:          []byte("testuserID"),
			cacheDeleteShouldBeCalled: true,
			repoDeleteShouldBeCalled:  true,
			wantAnyError:              true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			// setup
			testCtx, cancel := context.WithCancel(context.Background())
			defer cancel()

			repo, cache := tt.depSetup()

			testShortener := New(Dependencies{Repo: repo, Cache: cache}, Config{})

			tokenStr := "abcd"

			// execution
			gotError := testShortener.DeleteToken(testCtx, tokenStr, tt.requestingUserID)

			// assertions
			repo.(*mockRepo).AssertCalled(t, "GetToken", testCtx, tokenStr)
			if tt.cacheDeleteShouldBeCalled {
				cache.(*mockCache).AssertCalled(t, "DeleteToken", testCtx, tokenStr)
			} else {
				cache.(*mockCache).AssertNumberOfCalls(t, "DeleteToken", 0)
			}
			if tt.repoDeleteShouldBeCalled {
				repo.(*mockRepo).AssertCalled(t, "DeleteToken", testCtx, tokenStr)
			} else {
				repo.(*mockRepo).AssertNumberOfCalls(t, "DeleteToken", 0)
			}

			if tt.wantSpecificError != nil {
				assert.ErrorIs(t, gotError, tt.wantSpecificError)
			} else if tt.wantAnyError {
				assert.Error(t, gotError)
				assert.NotErrorIs(t, gotError, ErrNotTokenOwner)
			} else {
				assert.NoError(t, gotError)
			}
		})
	}

}

// mockRepo implements repository dependency interface using a testify/mock.Mock object.
type mockRepo struct {
	mock.Mock
}

func (m *mockRepo) HealthCheck(ctx context.Context) error {
	args := m.MethodCalled("HealthCheck", ctx)
	return args.Error(0)
}

func (m *mockRepo) SaveToken(ctx context.Context, token URLToken) error {
	args := m.MethodCalled("SaveToken", ctx, token)
	return args.Error(0)
}

func (m *mockRepo) GetToken(ctx context.Context, tokenStr string) (URLToken, error) {
	args := m.MethodCalled("GetToken", ctx, tokenStr)
	return args.Get(0).(URLToken), args.Error(1)
}

func (m *mockRepo) DeleteToken(ctx context.Context, tokenStr string) error {
	args := m.MethodCalled("DeleteToken", ctx, tokenStr)
	return args.Error(0)
}

// mockCache implements cache dependency interface using a testify/mock.Mock object.
type mockCache struct {
	mock.Mock
}

func (m *mockCache) HealthCheck(ctx context.Context) error {
	args := m.MethodCalled("HealthCheck", ctx)
	return args.Error(0)
}

func (m *mockCache) SaveToken(ctx context.Context, token Token, url URL, ttl time.Duration) error {
	args := m.MethodCalled("SaveToken", ctx, token, url, ttl)
	return args.Error(0)
}

func (m *mockCache) GetToken(ctx context.Context, tokenStr string) (URL, bool, error) {
	args := m.MethodCalled("GetToken", ctx, tokenStr)
	return args.Get(0).(URL), args.Bool(1), args.Error(2)
}

func (m *mockCache) DeleteToken(ctx context.Context, tokenStr string) error {
	args := m.MethodCalled("DeleteToken", ctx, tokenStr)
	return args.Error(0)
}

type mockNotUniqueError struct{}

func (m mockNotUniqueError) Error() string {
	return ""
}

func (m mockNotUniqueError) NotUnique() bool {
	return true
}

type mockNotFoundError struct{}

func (m mockNotFoundError) Error() string {
	return ""
}

func (m mockNotFoundError) NotFound() bool {
	return true
}
