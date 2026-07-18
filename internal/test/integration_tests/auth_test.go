package integration_tests

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/theokrutij/pet-urls/internal/db/postgres"
	"github.com/theokrutij/pet-urls/internal/services/auth"
	"golang.org/x/crypto/bcrypt"
)

// TestHealthCheck verifies that auth.HealthCheck returns no error
// when all its dependencies are healthy and accessible.
func TestAuthHealthcheck(t *testing.T) {
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	testAuth := auth.New(
		auth.Dependencies{
			Repo:    postgres.NewAuthRepository(pg),
			KeyFunc: func() []byte { return []byte(testJWTSecret) },
		},
		auth.Config{},
	)

	err := testAuth.HealthCheck(testCtx)
	assert.NoError(t, err)
}

func TestRegister(t *testing.T) {
	// setup
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)

	testAuth := auth.New(
		auth.Dependencies{
			Repo:    postgres.NewAuthRepository(pg),
			KeyFunc: func() []byte { return []byte(testJWTSecret) },
		},
		auth.Config{},
	)

	// execution
	err := testAuth.Register(testCtx, "testlogin", "testpassword")
	assert.NoError(t, err)

	// DB assertions
	q := `
		SELECT password_hash FROM users
		WHERE login = $1
	`
	var pwdHashInDB auth.PasswordHash
	err = pg.QueryRow(testCtx, q, "testlogin").Scan(&pwdHashInDB)
	assert.NoError(t, err)

	err = bcrypt.CompareHashAndPassword(pwdHashInDB, []byte("testpassword"))
	assert.NoError(t, err)
}

func TestLogin(t *testing.T) {
	// setup
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)
	pwdHash, _ := bcrypt.GenerateFromPassword([]byte("testpassword"), bcrypt.DefaultCost)
	q := `
		INSERT INTO users (login, password_hash)
		VALUES ($1, $2) 
		RETURNING id 
	`
	var userIDinDB auth.UserID
	err := pg.QueryRow(testCtx, q, "testlogin", pwdHash).Scan(&userIDinDB)
	assert.NoError(t, err)

	testAuth := auth.New(
		auth.Dependencies{
			Repo:    postgres.NewAuthRepository(pg),
			KeyFunc: func() []byte { return []byte(testJWTSecret) },
		},
		auth.Config{},
	)

	// execution
	gotRefreshToken, _, gotError := testAuth.Login(testCtx, "testlogin", "testpassword")
	assert.NoError(t, gotError)

	// DB assertions
	q = `
		SELECT user_id, expires_at FROM refresh_tokens
		WHERE token_hash = $1
	`
	var refreshTokenUserID auth.UserID
	var refreshTokenExp time.Time
	tokenHash := sha256.Sum256(gotRefreshToken)
	err = pg.QueryRow(testCtx, q, tokenHash[:]).Scan(&refreshTokenUserID, &refreshTokenExp)
	assert.NoError(t, err)
	assert.EqualValues(t, userIDinDB, refreshTokenUserID)
	assert.WithinDuration(t, time.Now().Add(testAuth.RefreshTokenTTL()), refreshTokenExp, acceptableTimePrecision)
}

func TestLogout(t *testing.T) {
	// setup
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)
	q := `
		INSERT INTO users (login, password_hash)
		VALUES ($1, $2) 
		RETURNING id 
	`
	var mockUserID auth.UserID
	err := pg.QueryRow(testCtx, q, "testlogin", []byte("testpasswordHash")).Scan(&mockUserID)
	assert.NoError(t, err)
	q = `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`
	mockRefreshToken := uuid.New()
	mockTokenHash := sha256.Sum256(mockRefreshToken[:])
	_, err = pg.Exec(testCtx, q, mockTokenHash[:], mockUserID, time.Now().Add(24*time.Hour))
	assert.NoError(t, err)

	testAuth := auth.New(
		auth.Dependencies{
			Repo:    postgres.NewAuthRepository(pg),
			KeyFunc: func() []byte { return []byte(testJWTSecret) },
		},
		auth.Config{},
	)

	// execution
	err = testAuth.Logout(testCtx, mockRefreshToken[:])
	assert.NoError(t, err)

	// DB assertions
	q = `
		SELECT revoked_at FROM refresh_tokens
		WHERE token_hash = $1
	`
	var dbRevokedAt time.Time
	err = pg.QueryRow(testCtx, q, mockTokenHash[:]).Scan(&dbRevokedAt)
	assert.NoError(t, err)
	assert.WithinDuration(t, time.Now(), dbRevokedAt, acceptableTimePrecision)
}

func TestRefresh(t *testing.T) {
	// setup
	testCtx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	pg := setupTestPostgres(testCtx, t)
	q := `
		INSERT INTO users (login, password_hash)
		VALUES ($1, $2) 
		RETURNING id 
	`
	var mockUserID auth.UserID
	err := pg.QueryRow(testCtx, q, "testlogin", []byte("testpasswordHash")).Scan(&mockUserID)
	assert.NoError(t, err)
	q = `
		INSERT INTO refresh_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`
	mockRefreshToken := uuid.New()
	mockTokenHash := sha256.Sum256(mockRefreshToken[:])
	_, err = pg.Exec(testCtx, q, mockTokenHash[:], mockUserID, time.Now().Add(24*time.Hour))
	assert.NoError(t, err)

	testAuth := auth.New(
		auth.Dependencies{
			Repo:    postgres.NewAuthRepository(pg),
			KeyFunc: func() []byte { return []byte(testJWTSecret) },
		},
		auth.Config{},
	)

	// execution
	_, err = testAuth.Refresh(testCtx, mockRefreshToken[:])

	// assertions
	assert.NoError(t, err)
}
