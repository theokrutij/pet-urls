package http

import (
	"encoding/base64"
	"net/http"
)

const (
	refreshTokenCookieName = "refreshToken"
	refreshTokenCookiePath = "/api/v1/auth"
)

func (s *server) setRefreshTokenCookie(w http.ResponseWriter, token []byte) {
	cookie := http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    base64.URLEncoding.EncodeToString(token),
		Path:     refreshTokenCookiePath,
		MaxAge:   int(s.auth.RefreshTokenTTL().Seconds()),
		Secure:   !s.debug, // allow cookies over http for debugging
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, &cookie)
}

func (s *server) expireRefreshTokenCookie(w http.ResponseWriter) {
	cookie := http.Cookie{
		Name:   refreshTokenCookieName,
		Value:  "",
		Path:   refreshTokenCookiePath,
		MaxAge: -1,
	}
	http.SetCookie(w, &cookie)
}

func getRefreshTokenFromCookie(r *http.Request) ([]byte, error) {
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		return nil, err
	}

	return base64.URLEncoding.DecodeString(cookie.Value)
}
