package server

import (
	"context"
	"net/http"
	"strings"

	"github.com/forward/forward/internal/auth"
)

type contextKey string

const userContextKey contextKey = "user"

type AuthUser struct {
	ID    string
	Email string
}

func userFromContext(ctx context.Context) (AuthUser, bool) {
	user, ok := ctx.Value(userContextKey).(AuthUser)
	return user, ok
}

func (s *HTTPServer) requireJWT(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			writeError(w, http.StatusUnauthorized, "требуется авторизация")
			return
		}

		claims, err := s.tokens.ParseAccessToken(token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "недействительный токен")
			return
		}

		ctx := context.WithValue(r.Context(), userContextKey, AuthUser{
			ID:    claims.UserID,
			Email: claims.Email,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, prefix))
}

func extractAPIToken(r *http.Request) string {
	if token := bearerToken(r.Header.Get("Authorization")); token != "" {
		return token
	}
	return strings.TrimSpace(r.URL.Query().Get("token"))
}

func (s *AgentServer) authenticateAgent(r *http.Request) (AuthUser, error) {
	plaintext := extractAPIToken(r)
	if plaintext == "" {
		return AuthUser{}, errUnauthorized
	}

	userID, err := s.apiTokens.GetUserIDByTokenHash(r.Context(), auth.HashAPIToken(plaintext))
	if err != nil {
		return AuthUser{}, errUnauthorized
	}

	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		return AuthUser{}, errUnauthorized
	}

	return AuthUser{ID: user.ID, Email: user.Email}, nil
}

var errUnauthorized = &authError{msg: "недействительный API token"}

type authError struct {
	msg string
}

func (e *authError) Error() string {
	return e.msg
}
