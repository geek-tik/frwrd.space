package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Tunnel struct {
	ID        string
	UserID    string
	Subdomain string
	LocalPort int
	Protocol  string
	Status    string
	StartedAt time.Time
	ClosedAt  *time.Time
}

type TunnelStore struct {
	pool *pgxpool.Pool
}

func NewTunnelStore(pool *pgxpool.Pool) *TunnelStore {
	return &TunnelStore{pool: pool}
}

func (s *TunnelStore) Create(ctx context.Context, userID, subdomain string, localPort int, protocol string) (Tunnel, error) {
	const q = `
		INSERT INTO tunnels (user_id, subdomain, local_port, protocol, status)
		VALUES ($1, $2, $3, $4, 'active')
		RETURNING id, user_id, subdomain, local_port, protocol, status, started_at, closed_at
	`

	var t Tunnel
	err := s.pool.QueryRow(ctx, q, userID, subdomain, localPort, protocol).Scan(
		&t.ID, &t.UserID, &t.Subdomain, &t.LocalPort, &t.Protocol, &t.Status, &t.StartedAt, &t.ClosedAt,
	)
	if err != nil {
		return Tunnel{}, fmt.Errorf("create tunnel: %w", err)
	}
	return t, nil
}

func (s *TunnelStore) Close(ctx context.Context, id string) error {
	const q = `
		UPDATE tunnels
		SET status = 'closed', closed_at = now()
		WHERE id = $1 AND status = 'active'
	`
	if _, err := s.pool.Exec(ctx, q, id); err != nil {
		return fmt.Errorf("close tunnel: %w", err)
	}
	return nil
}

func (s *TunnelStore) ListActiveByUser(ctx context.Context, userID string) ([]Tunnel, error) {
	const q = `
		SELECT id, user_id, subdomain, local_port, protocol, status, started_at, closed_at
		FROM tunnels
		WHERE user_id = $1 AND status = 'active'
		ORDER BY started_at DESC
	`

	rows, err := s.pool.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("list tunnels: %w", err)
	}
	defer rows.Close()

	var tunnels []Tunnel
	for rows.Next() {
		var t Tunnel
		if err := rows.Scan(
			&t.ID, &t.UserID, &t.Subdomain, &t.LocalPort, &t.Protocol, &t.Status, &t.StartedAt, &t.ClosedAt,
		); err != nil {
			return nil, fmt.Errorf("scan tunnel: %w", err)
		}
		tunnels = append(tunnels, t)
	}
	return tunnels, rows.Err()
}
