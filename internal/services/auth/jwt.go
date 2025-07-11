package auth

import (
	"errors"
	"time"
	"unicode/utf8"

	"github.com/golang-jwt/jwt/v5"
)

type JWTclaims struct {
	UserID    string
	ExpiresAt time.Time
}

func generateJWT(claims JWTclaims, keyFunc func() []byte) ([]byte, error) {
	if !utf8.ValidString(claims.UserID) {
		return nil, errors.New("generateJWT: UserID must be a valid utf-8 string")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt),
		Subject:   claims.UserID,
	})

	ss, err := token.SignedString(keyFunc())
	if err != nil {
		return nil, err
	}

	return []byte(ss), nil
}

func parseJWT(tokenCandidate string, keyFunc func() []byte) (UserID, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithExpirationRequired(),
	}
	kf := func(t *jwt.Token) (any, error) { return keyFunc(), nil }
	token, err := jwt.Parse(tokenCandidate, kf, opts...)
	if errors.Is(err, jwt.ErrTokenExpired) {
		return nil, errJWTExpired
	} else if err != nil {
		return nil, err
	}
	userIDAsBase64, err := token.Claims.GetSubject()
	if err != nil {
		return nil, err
	}
	userID, err := userIDfromBase64(userIDAsBase64)
	if err != nil {
		return nil, err
	}

	return userID, nil
}
