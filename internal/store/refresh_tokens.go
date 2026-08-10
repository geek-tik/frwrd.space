package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type RefreshTokenStore struct {
	pool *pgxpool.Pool
}

func NewRefreshTokenStore(pool *pgxpool.Pool) *RefreshTokenStore {
	return &RefreshTokenStore{pool: pool}
}

func (s *RefreshTokenStore) Create(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error {
	const q = `
		INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`
	if _, err := s.pool.Exec(ctx, q, userID, tokenHash, expiresAt); err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}

func (s *RefreshTokenStore) GetByHash(ctx context.Context, tokenHash string) (RefreshToken, error) {
	const q = `
		SELECT id, user_id, token_hash, expires_at, created_at
		FROM refresh_tokens
		WHERE token_hash = $1
	`

	var token RefreshToken
	err := s.pool.QueryRow(ctx, q, tokenHash).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.ExpiresAt, &token.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return RefreshToken{}, ErrNotFound
		}
		return RefreshToken{}, fmt.Errorf("get refresh token: %w", err)
	}
	return token, nil
}

func (s *RefreshTokenStore) DeleteByHash(ctx context.Context, tokenHash string) error {
	const q = `DELETE FROM refresh_tokens WHERE token_hash = $1`
	if _, err := s.pool.Exec(ctx, q, tokenHash); err != nil {
		return fmt.Errorf("delete refresh token: %w", err)
	}
	return nil
}

func (s *RefreshTokenStore) DeleteByUserID(ctx context.Context, userID string) error {
	const q = `DELETE FROM refresh_tokens WHERE user_id = $1`
	if _, err := s.pool.Exec(ctx, q, userID); err != nil {
		return fmt.Errorf("delete user refresh tokens: %w", err)
	}
	return nil
}
