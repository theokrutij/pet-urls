package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestUserIDFromJWT(t *testing.T) {
	mockUUID := uuid.New()
	mockUserID := UserID(mockUUID[:])
	mockKeyFunc := func() []byte { return []byte("mockKey") }

	claims := JWTclaims{
		UserID:    mockUserID.toBase64(),
		ExpiresAt: time.Now().Add(defaultAccessTokenTTL),
	}

	gotToken, err := generateJWT(claims, mockKeyFunc)

	assert.NoError(t, err)

	gotClaims, err := parseJWT(string(gotToken), mockKeyFunc)
	assert.NoError(t, err)

	userIDFromJWT := gotClaims.UserID
	assert.NoError(t, err)

	assert.Equal(t, mockUserID, UserID(userIDFromJWT))
}

func tamperJWTClaims(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		panic("invalid format")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		panic(err)
	}

	var claims map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		panic(err)
	}

	claims["sub"] = "hacked_user"

	// Re-encode payload
	newPayloadBytes, _ := json.Marshal(claims)
	newPayload := base64.RawURLEncoding.EncodeToString(newPayloadBytes)

	// Reassemble token (with original header and signature)
	return fmt.Sprintf("%s.%s.%s", parts[0], newPayload, parts[2])
}
