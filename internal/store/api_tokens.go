package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type APIToken struct {
	ID         string
	UserID     string
	TokenHash  string
	Name       string
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

type APITokenStore struct {
	pool *pgxpool.Pool
}

func NewAPITokenStore(pool *pgxpool.Pool) *APITokenStore {
	return &APITokenStore{pool: pool}
}

func (s *APITokenStore) Create(ctx context.Context, userID, tokenHash, name string) (APIToken, error) {
	const q = `
		INSERT INTO api_tokens (user_id, token_hash, name)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, name, last_used_at, created_at
	`

	var token APIToken
	err := s.pool.QueryRow(ctx, q, userID, tokenHash, name).Scan(
		&token.ID, &token.UserID, &token.TokenHash, &token.Name, &token.LastUsedAt, &token.CreatedAt,
	)
	if err != nil {
		return APIToken{}, fmt.Errorf("create api token: %w", err)
	}
	return token, nil
}

func (s *APITokenStore) ListByUserID(ctx context.Context, userID string) ([]APIToken, error) {
	const q = `
		SELECT id, user_id, token_hash, name, last_used_at, created_at
		FROM api_tokens
		WHERE user_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	var tokens []APIToken
	for rows.Next() {
		var token APIToken
		if err := rows.Scan(
			&token.ID, &token.UserID, &token.TokenHash, &token.Name, &token.LastUsedAt, &token.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		tokens = append(tokens, token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api tokens: %w", err)
	}
	return tokens, nil
}

func (s *APITokenStore) Delete(ctx context.Context, userID, tokenID string) error {
	const q = `DELETE FROM api_tokens WHERE id = $1 AND user_id = $2`
	tag, err := s.pool.Exec(ctx, q, tokenID, userID)
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *APITokenStore) GetUserIDByTokenHash(ctx context.Context, tokenHash string) (string, error) {
	const q = `
		UPDATE api_tokens
		SET last_used_at = now()
		WHERE token_hash = $1
		RETURNING user_id
	`

	var userID string
	err := s.pool.QueryRow(ctx, q, tokenHash).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("get user by api token: %w", err)
	}
	return userID, nil
}
