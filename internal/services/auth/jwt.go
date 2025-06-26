package auth

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

func generateJWT(claims claims, keyFunc func() []byte) ([]byte, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt),
		Subject:   string(claims.UserID),
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
		return nil, ErrTokenExpired
	} else if err != nil {
		return nil, err
	}

	userIDStr, err := token.Claims.GetSubject()
	if err != nil {
		return nil, err
	}

	return UserID(userIDStr), nil
}
