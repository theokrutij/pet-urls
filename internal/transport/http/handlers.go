package http

import (
	"encoding/base64"
	"errors"
	"net/http"
	"time"

	xcontext "github.com/theokrutij/pet-urls/internal/context"
	"github.com/theokrutij/pet-urls/internal/services/auth"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

func (s *server) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	if err := s.shortener.HealthCheck(r.Context()); err != nil {
		writeError(w, codeInternalError, "internal")
		return
	}

	if err := s.auth.HealthCheck(r.Context()); err != nil {
		writeError(w, codeInternalError, "internal")
	}
}

func (s *server) handleGenerateURLToken(w http.ResponseWriter, r *http.Request) {
	type requestSchema struct {
		OriginalURL string `json:"original_url"`
	}
	type responseSchema struct {
		ShortURL   string `json:"short_url"`
		ValidUntil string `json:"valid_until,omitempty"`
	}

	request, err := readBodyAsJSON[requestSchema](r)
	if err != nil {
		writeError(w, codeInvalidRequest, "not parsable")
		return
	}

	if request.OriginalURL == "" {
		writeError(w, codeInvalidParameter, "original_url empty or missing")
		return
	}

	token, err := s.shortener.GenerateToken(r.Context(), request.OriginalURL)
	if errors.Is(err, shortener.ErrInvalidURL) {
		writeError(w, codeInvalidParameter, "original_url not a valid HTTP URL")
		return
	} else if err != nil {
		writeError(w, codeInternalError, "internal")
		return
	}

	response := &responseSchema{ShortURL: string(token.Token)}

	// timestamp format: yyyy-mm-ddThh:mm:ssZ
	// time returned in the UTC tz
	if token.ExpiresAt != nil {
		response.ValidUntil = token.ExpiresAt.Format(time.RFC3339)
	}
	writeResponseAsJSON(w, response, 201)
}

func (s *server) handleResolveToken() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := tokenFromPath(r)
		if token == "" {
			http.Error(w, "", http.StatusNotFound)
		}

		originalURL, err := s.shortener.ResolveToken(r.Context(), token)
		if err != nil {
			http.Error(w, "unknown token", http.StatusNotFound)
			return
		}

		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, string(originalURL), http.StatusTemporaryRedirect)
	}
}

func tokenFromPath(r *http.Request) string {
	return r.PathValue("token")
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
		writeError(w, codeInvalidRequest, "expected application/x-www-form-urlencoded content-type")
		return
	}
	if err := r.ParseForm(); err != nil {
		writeError(w, codeInvalidRequest, "unparsable form")
		return
	}

	login := r.PostForm.Get("login")
	if login == "" {
		writeError(w, codeInvalidParameter, "login empty or missing")
		return
	}
	password := r.PostForm.Get("password")
	if password == "" {
		writeError(w, codeInvalidParameter, "password empty or missing")
		return
	}

	refreshToken, accessToken, err := s.auth.Login(r.Context(), login, password)
	if errors.Is(err, auth.ErrLoginDoesNotExist) || errors.Is(err, auth.ErrPasswordDoesNotMatch) {
		writeError(w, codeUnauthenticated, "invalid credentials")
		return
	} else if err != nil {
		writeError(w, codeInternalError, "internal")
		return
	}

	s.setRefreshTokenCookie(w, refreshToken)
	writeAccessTokenAsJSON(w, accessToken)
}

func writeAccessTokenAsJSON(w http.ResponseWriter, token auth.AccessToken) {
	var response = struct {
		AccessToken string `json:"access_token"`
	}{
		AccessToken: string(token),
	}
	writeResponseAsJSON(w, response, 200)
}

const (
	refreshTokenCookieName = "refreshToken"
)

func (s *server) setRefreshTokenCookie(w http.ResponseWriter, token []byte) {
	cookie := http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    base64.URLEncoding.EncodeToString(token),
		Path:     "/api",
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
		Path:   "/api",
		MaxAge: -1,
	}
	http.SetCookie(w, &cookie)
}

func (s *server) handleRegistration(w http.ResponseWriter, r *http.Request) {
	type requestSchema struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}

	request, err := readBodyAsJSON[requestSchema](r)
	if err != nil {
		writeError(w, codeInvalidRequest, "not parsable")
		return
	}
	if request.Login == "" {
		writeError(w, codeInvalidParameter, "login empty or missing")
		return
	}
	if request.Password == "" {
		writeError(w, codeInvalidParameter, "password empty or missing")
		return
	}

	refreshToken, accessToken, err := s.auth.Register(r.Context(), request.Login, request.Password)
	if errors.Is(err, auth.ErrInvalidLogin) {
		writeError(w, codeInvalidParameter, "invalid login")
		return
	} else if errors.Is(err, auth.ErrInvalidPassword) {
		writeError(w, codeInvalidParameter, "invalid password")
		return
	} else if errors.Is(err, auth.ErrLoginNotUnique) {
		writeError(w, codeInvalidParameter, "login already exists")
		return
	} else if err != nil {
		writeError(w, codeInternalError, "")
		return
	}

	s.setRefreshTokenCookie(w, refreshToken)
	writeAccessTokenAsJSON(w, accessToken)
}

func (s *server) handleTokenRefresh(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := getRefreshTokenFromCookie(r)
	if err != nil {
		writeError(w, codeUnauthenticated, "unauthenticated")
		return
	}

	accessToken, err := s.auth.Refresh(r.Context(), refreshToken)
	if err != nil {
		writeError(w, codeUnauthenticated, "unauthenticated")
		return
	}

	writeAccessTokenAsJSON(w, accessToken)
}

func getRefreshTokenFromCookie(r *http.Request) ([]byte, error) {
	cookie, err := r.Cookie(refreshTokenCookieName)
	if err != nil {
		return nil, err
	}

	return base64.URLEncoding.DecodeString(cookie.Value)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	refreshToken, err := getRefreshTokenFromCookie(r)
	if err != nil {
		writeError(w, codeUnauthenticated, "unauthenticated")
		return
	}

	err = s.auth.Logout(r.Context(), refreshToken)
	if err != nil {
		writeError(w, codeInternalError, "internal")
		return
	}

	s.expireRefreshTokenCookie(w)
}

func (s *server) handleCreateTokenWithOwner(w http.ResponseWriter, r *http.Request) {
	type requestSchema struct {
		Token string `json:"token"`
		URL   string `json:"url"`
		TTL   int    `json:"ttl"`
	}
	type responseSchema struct {
		Token     shortener.Token `json:"token"`
		ExpiresAt time.Time       `json:"expires_at"`
	}

	request, err := readBodyAsJSON[requestSchema](r)
	if err != nil {
		writeError(w, codeInvalidRequest, "unparsable")
		return
	}

	userID, ok := xcontext.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, codeUnauthenticated, "unauthenticated")
		return
	}

	input := shortener.CreateTokenInput{
		URL:     request.URL,
		Token:   request.Token,
		TTL:     time.Duration(request.TTL),
		OwnerID: userID,
	}
	token, err := s.shortener.CreateTokenWithOwner(r.Context(), input)
	if errors.Is(err, shortener.ErrInvalidURL) {
		writeError(w, codeInvalidParameter, "url is not a valid HTTP url")
		return
	} else if errors.Is(err, shortener.ErrInvalidToken) {
		writeError(w, codeInvalidParameter, "token must contain at most 64 characters")
		return
	} else if errors.Is(err, shortener.ErrTokenIsNotUnique) {
		writeError(w, codeInvalidParameter, "token already exists")
		return
	} else if err != nil {
		writeError(w, codeInternalError, "internal")
		return
	}

	response := responseSchema{
		Token:     token.Token,
		ExpiresAt: *token.ExpiresAt,
	}

	writeResponseAsJSON(w, response, 201)
}

func (s *server) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	token := tokenFromPath(r)
	if token == "" {
		return
	}

	userID, ok := xcontext.UserIDFromContext(r.Context())
	if !ok {
		writeError(w, codeUnauthenticated, "unauthenticated")
		return
	}

	err := s.shortener.DeleteToken(r.Context(), token, userID)
	if errors.Is(err, shortener.ErrNotTokenOwner) {
		writeError(w, codeMustBeOwner, "you don't own this token")
		return
	} else if err != nil {
		writeError(w, codeInternalError, "internal")
	}
}
