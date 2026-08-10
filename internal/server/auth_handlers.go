package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/forward/forward/internal/auth"
	"github.com/forward/forward/internal/store"
)

const refreshCookieName = "forward_refresh"

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type createTokenRequest struct {
	Name string `json:"name"`
}

type authResponse struct {
	AccessToken string    `json:"access_token"`
	ExpiresIn   int       `json:"expires_in"`
	User        userView  `json:"user"`
}

type userView struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type tokenView struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type createTokenResponse struct {
	tokenView
	Token string `json:"token"`
}

func (s *HTTPServer) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !isValidEmail(email) {
		writeError(w, http.StatusBadRequest, "некорректный email")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "пароль должен быть не короче 8 символов")
		return
	}

	passwordHash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать пользователя")
		return
	}

	user, err := s.users.Create(r.Context(), email, passwordHash)
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "пользователь с таким email уже существует")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось создать пользователя")
		return
	}

	s.issueSession(w, r, user)
}

func (s *HTTPServer) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, err := s.users.GetByEmail(r.Context(), email)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "неверный email или пароль")
			return
		}
		writeError(w, http.StatusInternalServerError, "ошибка авторизации")
		return
	}

	if err := auth.CheckPassword(user.PasswordHash, req.Password); err != nil {
		writeError(w, http.StatusUnauthorized, "неверный email или пароль")
		return
	}

	s.issueSession(w, r, user)
}

func (s *HTTPServer) refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(refreshCookieName)
	if err != nil || cookie.Value == "" {
		writeError(w, http.StatusUnauthorized, "refresh token не найден")
		return
	}

	stored, err := s.refreshTokens.GetByHash(r.Context(), auth.HashAPIToken(cookie.Value))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusUnauthorized, "недействительный refresh token")
			return
		}
		writeError(w, http.StatusInternalServerError, "ошибка обновления сессии")
		return
	}

	if time.Now().After(stored.ExpiresAt) {
		_ = s.refreshTokens.DeleteByHash(r.Context(), stored.TokenHash)
		clearRefreshCookie(w)
		writeError(w, http.StatusUnauthorized, "сессия истекла")
		return
	}

	user, err := s.users.GetByID(r.Context(), stored.UserID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "пользователь не найден")
		return
	}

	accessToken, err := s.tokens.IssueAccessToken(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось выдать access token")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"access_token": accessToken,
		"expires_in":   int(s.tokens.AccessTTL().Seconds()),
	})
}

func (s *HTTPServer) logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(refreshCookieName); err == nil && cookie.Value != "" {
		_ = s.refreshTokens.DeleteByHash(r.Context(), auth.HashAPIToken(cookie.Value))
	}
	clearRefreshCookie(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) me(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется авторизация")
		return
	}
	writeJSON(w, http.StatusOK, userView{ID: user.ID, Email: user.Email})
}

func (s *HTTPServer) listTokens(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	tokens, err := s.apiTokens.ListByUserID(r.Context(), user.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить токены")
		return
	}

	result := make([]tokenView, 0, len(tokens))
	for _, token := range tokens {
		result = append(result, tokenView{
			ID:         token.ID,
			Name:       token.Name,
			LastUsedAt: token.LastUsedAt,
			CreatedAt:  token.CreatedAt,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"tokens": result})
}

func (s *HTTPServer) createToken(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	var req createTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && r.ContentLength > 0 {
		writeError(w, http.StatusBadRequest, "некорректный JSON")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "default"
	}

	plaintext, hash, err := auth.GenerateAPIToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось создать токен")
		return
	}

	token, err := s.apiTokens.Create(r.Context(), user.ID, hash, name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить токен")
		return
	}

	writeJSON(w, http.StatusCreated, createTokenResponse{
		tokenView: tokenView{
			ID:        token.ID,
			Name:      token.Name,
			CreatedAt: token.CreatedAt,
		},
		Token: plaintext,
	})
}

func (s *HTTPServer) deleteToken(w http.ResponseWriter, r *http.Request) {
	user, ok := userFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "требуется авторизация")
		return
	}

	tokenID := chi.URLParam(r, "id")
	if err := s.apiTokens.Delete(r.Context(), user.ID, tokenID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeError(w, http.StatusNotFound, "токен не найден")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось удалить токен")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *HTTPServer) issueSession(w http.ResponseWriter, r *http.Request, user store.User) {
	accessToken, err := s.tokens.IssueAccessToken(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось выдать access token")
		return
	}

	refreshPlain, refreshHash, err := auth.GenerateRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось выдать refresh token")
		return
	}

	expiresAt := time.Now().Add(s.tokens.RefreshTTL())
	if err := s.refreshTokens.Create(r.Context(), user.ID, refreshHash, expiresAt); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить сессию")
		return
	}

	setRefreshCookie(w, refreshPlain, expiresAt, s.secureCookies)

	writeJSON(w, http.StatusOK, authResponse{
		AccessToken: accessToken,
		ExpiresIn:   int(s.tokens.AccessTTL().Seconds()),
		User:        userView{ID: user.ID, Email: user.Email},
	})
}

func setRefreshCookie(w http.ResponseWriter, value string, expiresAt time.Time, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    value,
		Path:     "/api/v1/auth",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
	})
}

func clearRefreshCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookieName,
		Value:    "",
		Path:     "/api/v1/auth",
		HttpOnly: true,
		MaxAge:   -1,
	})
}

func isValidEmail(email string) bool {
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}
