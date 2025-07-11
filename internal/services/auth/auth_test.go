package auth

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	validLogin        = "testlogin"
	validPassword     = "testpassword"
	validRefreshToken = "refreshtoken"

	acceptableTimePrecision = time.Second
)

// TestHealthCheck verifies that the Healthcheck method
// calls repo.HealthCheck and either
// returns an error if repo returns an error
// or returns nil if repo return nil.
func TestHealthCheck(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
	}{
		{
			name: "nil from repository",
		},
		{
			name:    "error from repository",
			repoErr: errors.New("repo: something went wrong"),
		},
	}

	for _, tt := range tests {
		// setup
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		repo := new(mockRepo)
		repo.On("HealthCheck", mock.Anything).Return(tt.repoErr)

		testAuth := New(
			Dependencies{Repo: repo, KeyFunc: mockKeyFunc},
			Config{},
		)

		// execution
		gotErr := testAuth.HealthCheck(testCtx)

		// assertions
		repo.AssertCalled(t, "HealthCheck", testCtx)

		if tt.repoErr != nil {
			assert.Error(t, gotErr)
		} else {
			assert.NoError(t, gotErr)
		}
	}
}

// TestRegister verifies that the Register method:
//   - Validates the login and password, returning appropriate errors for invalid inputs.
//   - Hashes the password before passing it to the repository.
//   - Calls repo.SaveUser with the login and password hash if validation passes.
//   - Wraps and returns specific error values for known repository errors (e.g., not unique).
//   - Returns a generic error if repo returns an unexpected error.
func TestRegister(t *testing.T) {
	tests := []struct {
		name           string
		login          string
		password       string
		repoErr        error
		wantErr        error
		wantGenericErr bool
		shouldCallRepo bool
	}{
		{
			name:           "valid login and password, nil from repo",
			login:          validLogin,
			password:       validPassword,
			shouldCallRepo: true,
		},
		{
			name:           "valid login and password, non-unique err from repo",
			login:          validLogin,
			password:       validPassword,
			repoErr:        mockNotUniqueError{},
			wantErr:        ErrLoginNotUnique,
			shouldCallRepo: true,
		},
		{
			name:           "long login, nil from repo",
			login:          strings.Repeat("a", 64),
			password:       validPassword,
			shouldCallRepo: true,
		},
		{
			name:     "too long login, nil from repo",
			login:    strings.Repeat("a", 65),
			password: validPassword,
			wantErr:  ErrInvalidLogin,
		},
		{
			name:           "long password, nil from repo",
			login:          validLogin,
			password:       strings.Repeat("a", 72),
			shouldCallRepo: true,
		},
		{
			name:     "too long password, nil from repo",
			login:    validLogin,
			password: strings.Repeat("a", 73),
			wantErr:  ErrInvalidPassword,
		},
		{
			name:           "valid login and password, unknown error from repo",
			login:          validLogin,
			password:       validPassword,
			shouldCallRepo: true,
			repoErr:        errors.New("something went wrong in repo"),
			wantGenericErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// setup
			testCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			repo := new(mockRepo)
			var capturedPwdHash []byte
			repo.
				On("SaveUser", testCtx, tt.login, mock.Anything).
				Run(func(args mock.Arguments) {
					capturedPwdHash = args.Get(2).(PasswordHash)
				}).
				Return(UserID([]byte("abcd")), tt.repoErr)

			testAuth := New(
				Dependencies{Repo: repo, KeyFunc: mockKeyFunc},
				Config{},
			)

			// execution
			gotErr := testAuth.Register(testCtx, tt.login, tt.password)

			// assertions
			if tt.wantErr != nil {
				assert.ErrorIs(t, gotErr, tt.wantErr)
			} else if tt.wantGenericErr {
				assert.Error(t, gotErr)
				assert.NotErrorIs(t, gotErr, ErrLoginNotUnique)
				assert.NotErrorIs(t, gotErr, ErrInvalidLogin)
				assert.NotErrorIs(t, gotErr, ErrInvalidPassword)
			} else {
				assert.NoError(t, gotErr)

			}

			if tt.shouldCallRepo {
				repo.AssertCalled(t, "SaveUser", testCtx, tt.login, mock.Anything)
				assert.NotEqualValues(t, tt.password, capturedPwdHash, "must not pass password to repo without hashing")
			} else {
				repo.AssertNotCalled(t, "SaveUser")
			}
		})
	}
}

// TestLogin verifies that the Login method:
//   - Calls repo.GetUser with provided login
//   - Returns ErrInvalidCredentials if the login doesn't exist or the password hashes don't match.
//   - Calls repo.SaveRefreshToken, hashing refresh token before storage.
//   - Returns a token pair (refresh and access) if authentication succeeds.
//   - Returns a generic error if repo returns an unexpected error.
func TestLogin(t *testing.T) {
	tests := []struct {
		name                string
		login               string
		password            string
		getUserErr          error
		shouldCallSaveToken bool
		saveTokenErr        error
		wantErr             error
		wantGenericErr      bool
	}{
		{
			name:                "valid login and password, nil error from repo",
			login:               validLogin,
			password:            validPassword,
			shouldCallSaveToken: true,
		},
		{
			name:       "non-existing login",
			login:      "somelogin",
			password:   validPassword,
			getUserErr: mockNotFoundError{},
			wantErr:    ErrInvalidCredentials,
		},
		{
			name:     "incorrect password",
			login:    validLogin,
			password: validPassword + "heheXD",
			wantErr:  ErrInvalidCredentials,
		},
		{
			name:           "unexpected error from repo.GetUser",
			login:          validLogin,
			password:       validPassword,
			getUserErr:     errors.New("something went wrong"),
			wantGenericErr: true,
		},
		{
			name:           "unexpected error from repo.SaveRefreshToken",
			login:          validLogin,
			password:       validPassword,
			saveTokenErr:   errors.New("something went wrong"),
			wantGenericErr: true,
		},
	}

	for _, tt := range tests {
		// setup
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockUUID := uuid.New()
		mockUserID := UserID(mockUUID[:])
		pwdHash, _ := hashPassword(validPassword)

		repo := new(mockRepo)
		repo.On("GetUser", testCtx, tt.login).Return(
			UserInRepo{
				ID:           mockUserID,
				Login:        tt.login,
				PasswordHash: pwdHash,
			},
			tt.getUserErr,
		)
		var capturedTokenHash TokenHash
		repo.
			On("SaveRefreshToken", mock.Anything, mock.Anything).
			Run(func(args mock.Arguments) { capturedTokenHash = args.Get(1).(RefreshTokenInRepo).Hash }).
			Return(tt.saveTokenErr)

		testAuth := New(
			Dependencies{Repo: repo, KeyFunc: mockKeyFunc},
			Config{},
		)

		// execution
		gotRefreshToken, gotAccessToken, gotErr := testAuth.Login(testCtx, tt.login, tt.password)

		// assertions

		repo.AssertCalled(t, "GetUser", testCtx, tt.login)
		if tt.shouldCallSaveToken {
			repo.AssertCalled(t, "SaveRefreshToken", testCtx, mock.MatchedBy(func(t RefreshTokenInRepo) bool {
				ok := bytes.Equal(t.UserID, mockUserID)
				ok = ok && time.Duration.Abs(time.Until(t.ExpiresAt)-defaultRefreshTokenTTL) < acceptableTimePrecision
				return ok
			}))
		}

		if tt.wantErr != nil {
			assert.ErrorIs(t, gotErr, tt.wantErr)
		} else if tt.wantGenericErr {
			assert.Error(t, gotErr)

			assert.NotErrorIs(t, gotErr, ErrInvalidCredentials)
		} else {
			assert.NoError(t, gotErr)

			assert.Equal(t, capturedTokenHash, hashToken(gotRefreshToken))

			claims, err := parseJWT(string(gotAccessToken), mockKeyFunc)
			assert.NoError(t, err)

			userIDFromAccessToken, err := userIDfromBase64(claims.UserID)
			assert.NoError(t, err)

			assert.Equal(t, mockUserID, userIDFromAccessToken)
		}
	}
}

// TestLogout verifies that the Logout method:
//   - Calls repo.RevokeRefreshToken with a hash of provided token and returns nil if repo returns nil.
//   - Returns a generic error if repo returns an unexpected error.
func TestLogout(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		wantErr bool
	}{
		{
			name: "no error from repo",
		},
		{
			name:    "generic error from repo",
			repoErr: errors.New("something went wrong"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		//setup
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tokenCandidate := []byte(validRefreshToken)

		repo := new(mockRepo)
		repo.On("RevokeRefreshToken", testCtx, hashToken(tokenCandidate)).Return(tt.repoErr)

		testAuth := New(Dependencies{Repo: repo, KeyFunc: mockKeyFunc}, Config{})

		// execution
		err := testAuth.Logout(testCtx, tokenCandidate)

		// assertions
		repo.AssertCalled(t, "RevokeRefreshToken", testCtx, hashToken(tokenCandidate))

		if tt.wantErr {
			assert.Error(t, err)
		} else {
			assert.NoError(t, err)
		}
	}

}

// TestRefresh verifies that the Refresh method:
//   - Calls repo.GetRefreshToken with a hash of provided token.
//   - Returns ErrInvalidToken if the hash doesn't exist in repo
//   - Returns ErrInvalidToken if the token was found in repo, but it is expired or revoked.
//   - Generates and returns an access token that references the same UserID as the refresh token from repo.
//   - Returns a generic error if repo returns an unexpected error.
func TestRefresh(t *testing.T) {
	tests := []struct {
		name               string
		repoErr            error
		tokenInRepoExpired bool
		tokenInRepoRevoked bool
		wantErr            error
		wantGenericErr     bool
	}{
		{
			name: "nil error from repo",
		},
		{
			name:    "token doesn't exist in repo",
			repoErr: mockNotFoundError{},
			wantErr: ErrInvalidToken,
		},
		{
			name:               "token expired",
			tokenInRepoExpired: true,
			wantErr:            ErrInvalidToken,
		},
		{
			name:               "token revoked",
			tokenInRepoRevoked: true,
			wantErr:            ErrInvalidToken,
		},
		{
			name:           "unexpected error from repo",
			repoErr:        errors.New("something went wrong"),
			wantGenericErr: true,
		},
	}

	for _, tt := range tests {
		// setup
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tokenCandidate := []byte(validRefreshToken)
		mockUUID := uuid.New()
		mockUserID := UserID(mockUUID[:])
		tokenInRepo := RefreshTokenInRepo{}
		if tt.repoErr == nil {
			tokenInRepo.Hash = hashToken(tokenCandidate)
			tokenInRepo.UserID = mockUserID
			if tt.tokenInRepoExpired {
				tokenInRepo.ExpiresAt = time.Now().Add(-1 * acceptableTimePrecision)
			} else {
				tokenInRepo.ExpiresAt = time.Now().Add(acceptableTimePrecision)
			}
			if tt.tokenInRepoRevoked {
				revocationTime := time.Now().Add(-1 * acceptableTimePrecision)
				tokenInRepo.RevokedAt = &revocationTime
			}
		}

		repo := new(mockRepo)
		repo.On("GetRefreshToken", mock.Anything, mock.Anything).Return(tokenInRepo, tt.repoErr)

		testAuth := New(
			Dependencies{Repo: repo, KeyFunc: mockKeyFunc},
			Config{},
		)

		// execution
		gotAccessToken, gotErr := testAuth.Refresh(testCtx, tokenCandidate)

		// assertions
		repo.AssertCalled(t, "GetRefreshToken", testCtx, hashToken(tokenCandidate))

		if tt.wantErr != nil {
			assert.ErrorIs(t, gotErr, tt.wantErr)
		} else if tt.wantGenericErr {
			assert.Error(t, gotErr)
			assert.NotErrorIs(t, gotErr, ErrInvalidToken)
		} else {
			assert.NoError(t, gotErr)

			claims, err := parseJWT(string(gotAccessToken), mockKeyFunc)
			assert.NoError(t, err)

			userIDFromAccessToken, err := userIDfromBase64(claims.UserID)
			assert.NoError(t, err)

			assert.Equal(t, mockUserID, userIDFromAccessToken)
		}

	}

}

// TestAuthenticate verifies that the Authenticate method:
//   - Parses tokenCandidate as JWT
//   - If token is malformed, tampered or signed with a different key, returns ErrInvalidToken
//   - If exp claim contains a time in the past, returns ErrInvalidToken
//   - If token is valid, parses sub claim as base64 and returns it with nil error
func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name                string
		setupTokenCandidate func(userID UserID) []byte
		wantErr             error
	}{
		{
			name: "valid token",
			setupTokenCandidate: func(userID UserID) []byte {
				jwt, _ := generateJWT(
					JWTclaims{
						UserID:    userID.toBase64(),
						ExpiresAt: time.Now().Add(defaultAccessTokenTTL),
					},
					mockKeyFunc,
				)
				return jwt
			},
		},
		{
			name: "malformed token",
			setupTokenCandidate: func(userID UserID) []byte {
				jwt, _ := generateJWT(
					JWTclaims{
						UserID:    userID.toBase64(),
						ExpiresAt: time.Now().Add(defaultAccessTokenTTL),
					},
					mockKeyFunc,
				)

				jwt = append([]byte("abcd"), jwt...)
				jwt = append(jwt, []byte("abcd")...)

				return jwt
			},
			wantErr: ErrInvalidToken,
		},
		{
			name: "tampered token",
			setupTokenCandidate: func(userID UserID) []byte {
				jwt, _ := generateJWT(
					JWTclaims{
						UserID:    userID.toBase64(),
						ExpiresAt: time.Now().Add(defaultAccessTokenTTL),
					},
					mockKeyFunc,
				)

				tampered := tamperJWTClaims(string(jwt))
				return []byte(tampered)
			},
			wantErr: ErrInvalidToken,
		},
		{
			name: "expired token",
			setupTokenCandidate: func(userID UserID) []byte {
				jwt, _ := generateJWT(
					JWTclaims{
						UserID:    userID.toBase64(),
						ExpiresAt: time.Now().Add(-1 * acceptableTimePrecision),
					},
					mockKeyFunc,
				)

				return jwt
			},
			wantErr: ErrInvalidToken,
		},
	}

	for _, tt := range tests {
		// setup
		testCtx, cancel := context.WithCancel(context.Background())
		defer cancel()

		mockUUID := uuid.New()
		mockUserID := UserID(mockUUID[:])

		tokenCandidate := tt.setupTokenCandidate(mockUserID)

		testAuth := New(Dependencies{&mockRepo{}, mockKeyFunc}, Config{})

		// execution
		userID, gotErr := testAuth.Authenticate(testCtx, tokenCandidate)

		// assertions
		if tt.wantErr != nil {
			assert.ErrorIs(t, gotErr, tt.wantErr)
		} else {
			assert.NoError(t, gotErr)

			assert.Equal(t, mockUserID, userID)
		}
	}
}

// TestRefreshTokenTTL verifies that the RefreshTokenTTL method
// returns the same TTL value that was provided during Auth construction.
func TestRefreshTokenTTL(t *testing.T) {
	// setup
	mockTTL := 24 * 365 * 100 * time.Hour
	testAuth := New(Dependencies{&mockRepo{}, mockKeyFunc}, Config{RefreshTokenTTL: mockTTL})

	// execution
	gotTTL := testAuth.RefreshTokenTTL()

	// assertion
	assert.Equal(t, mockTTL, gotTTL)
}

// mockRepo implements repository dependency interface using a testify/mock.Mock object.
type mockRepo struct {
	mock.Mock
}

func (m *mockRepo) HealthCheck(ctx context.Context) error {
	args := m.MethodCalled("HealthCheck", ctx)
	return args.Error(0)
}

func (m *mockRepo) SaveUser(ctx context.Context, user UserInRepo) (UserID, error) {
	args := m.MethodCalled("SaveUser", ctx, user.Login, user.PasswordHash)
	return args.Get(0).(UserID), args.Error(1)
}

func (m *mockRepo) GetUser(ctx context.Context, login string) (UserInRepo, error) {
	args := m.MethodCalled("GetUser", ctx, login)
	return args.Get(0).(UserInRepo), args.Error(1)
}

func (m *mockRepo) SaveRefreshToken(ctx context.Context, token RefreshTokenInRepo) error {
	args := m.MethodCalled("SaveRefreshToken", ctx, token)
	return args.Error(0)
}

func (m *mockRepo) GetRefreshToken(ctx context.Context, tokenHash TokenHash) (RefreshTokenInRepo, error) {
	args := m.MethodCalled("GetRefreshToken", ctx, tokenHash)
	return args.Get(0).(RefreshTokenInRepo), args.Error(1)
}

func (m *mockRepo) RevokeRefreshToken(ctx context.Context, tokenHash TokenHash) error {
	args := m.MethodCalled("RevokeRefreshToken", ctx, tokenHash)
	return args.Error(0)
}

func mockKeyFunc() []byte {
	return []byte("abcd")
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
