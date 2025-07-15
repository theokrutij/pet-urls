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

func generateJWT(claims JWTclaims, keyFunc func() []byte) (string, error) {
	if !utf8.ValidString(claims.UserID) {
		return "", errors.New("generateJWT: UserID must be a valid utf-8 string")
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt),
		Subject:   claims.UserID,
	})

	ss, err := token.SignedString(keyFunc())
	if err != nil {
		return "", err
	}

	return ss, nil
}

func parseJWT(tokenCandidate string, keyFunc func() []byte) (*JWTclaims, error) {
	opts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithoutClaimsValidation(),
	}
	kf := func(t *jwt.Token) (any, error) { return keyFunc(), nil }
	token, err := jwt.Parse(tokenCandidate, kf, opts...)
	if err != nil {
		return nil, err
	}

	output := new(JWTclaims)

	userIDAsBase64, err := token.Claims.GetSubject()
	if err != nil {
		return nil, err
	}
	output.UserID = userIDAsBase64
	exp, err := token.Claims.GetExpirationTime()
	if err != nil {
		return nil, err
	}
	output.ExpiresAt = exp.Time

	return output, nil
}
