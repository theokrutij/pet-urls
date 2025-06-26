package http

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/theokrutij/pet-urls/internal/services/auth"
	"github.com/theokrutij/pet-urls/internal/services/shortener"
)

// simple healthcheck
// should also healthcheck shortener (maybe as a separate endpoint)
func (s *server) handleHealthcheck(w http.ResponseWriter, r *http.Request) {
	if err := s.shortener.HealthCheck(r.Context()); err != nil {
		writeError(w, codeInternalError, fmt.Sprintf("server, healthcheck: %s", err))
	}
}

// handleCreateShortURL attempts to read the original URL from the request body
// and calls shortener.GenerateCode to create a short URL code.
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

// should this piece of logic be split into two?
// one would handle custom redirect-related logic, the other would talk to the shortener
// TODO:
//   - figure out client side caching: appropriate headers and status code for that?
func (s *server) redirectToOriginalURL() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("code")
		if token == "" {
			http.Error(w, "", http.StatusNotFound)
		}

		originalURL, err := s.shortener.ResolveToken(r.Context(), token)
		if err == nil {
			http.Redirect(w, r, string(originalURL), http.StatusMovedPermanently)
			return
		}
		http.Error(w, "unknown token", http.StatusNotFound)
	}
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
		writeError(w, codeInvalidCredentials, "invalid credentials")
		return
	} else if err != nil {
		writeError(w, codeInternalError, "internal")
		return
	}

	setRefreshTokenCookie(w, refreshToken)
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

// TODO: move to config
const (
	refreshTokenCookieName = "refreshToken"
	refreshTokenMaxAge     = 7 * 24 * time.Hour
)

func setRefreshTokenCookie(w http.ResponseWriter, token []byte) {
	cookie := http.Cookie{
		Name:     refreshTokenCookieName,
		Value:    base64.URLEncoding.EncodeToString(token),
		MaxAge:   int(refreshTokenMaxAge.Seconds()),
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
	http.SetCookie(w, &cookie)
}

func (s *server) handleRegistration(w http.ResponseWriter, r *http.Request) { // TODO: do we need config?
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

	setRefreshTokenCookie(w, refreshToken)
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
	}
}
