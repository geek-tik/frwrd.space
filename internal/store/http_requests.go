package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type HTTPRequest struct {
	ID          string
	TunnelID    string
	Method      string
	Path        string
	Query       string
	Headers     []byte
	Body        []byte
	StatusCode  int
	RespHeaders []byte
	RespBody    []byte
	DurationMs  int
	CreatedAt   time.Time
}

type HTTPRequestInput struct {
	TunnelID    string
	Method      string
	Path        string
	Query       string
	Headers     []byte
	Body        []byte
	StatusCode  int
	RespHeaders []byte
	RespBody    []byte
	DurationMs  int
}

type HTTPRequestStore struct {
	pool *pgxpool.Pool
}

func NewHTTPRequestStore(pool *pgxpool.Pool) *HTTPRequestStore {
	return &HTTPRequestStore{pool: pool}
}

func MarshalHeaders(h http.Header) ([]byte, error) {
	m := make(map[string]string, len(h))
	for k, values := range h {
		if len(values) == 0 {
			continue
		}
		out := values[0]
		for i := 1; i < len(values); i++ {
			out += ", " + values[i]
		}
		m[k] = out
	}
	return json.Marshal(m)
}

func UnmarshalHeaders(raw []byte) map[string]string {
	if len(raw) == 0 {
		return map[string]string{}
	}
	var m map[string]string
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]string{}
	}
	return m
}

func (s *HTTPRequestStore) Create(ctx context.Context, in HTTPRequestInput) (HTTPRequest, error) {
	const q = `
		INSERT INTO http_requests (
			tunnel_id, method, path, query, headers, body,
			status_code, resp_headers, resp_body, duration_ms
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, tunnel_id, method, path, query, headers, body,
		          status_code, resp_headers, resp_body, duration_ms, created_at
	`

	var row HTTPRequest
	var query sql.NullString
	err := s.pool.QueryRow(ctx, q,
		in.TunnelID, in.Method, in.Path, nullIfEmpty(in.Query), in.Headers, in.Body,
		in.StatusCode, in.RespHeaders, in.RespBody, in.DurationMs,
	).Scan(
		&row.ID, &row.TunnelID, &row.Method, &row.Path, &query,
		&row.Headers, &row.Body, &row.StatusCode, &row.RespHeaders, &row.RespBody,
		&row.DurationMs, &row.CreatedAt,
	)
	if err != nil {
		return HTTPRequest{}, fmt.Errorf("create http request: %w", err)
	}
	row.Query = query.String
	return row, nil
}

func (s *HTTPRequestStore) ListByTunnel(ctx context.Context, tunnelID string, limit int) ([]HTTPRequest, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `
		SELECT id, tunnel_id, method, path, query, headers, body,
		       status_code, resp_headers, resp_body, duration_ms, created_at
		FROM http_requests
		WHERE tunnel_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	return s.scanMany(ctx, q, tunnelID, limit)
}

func (s *HTTPRequestStore) GetByID(ctx context.Context, tunnelID, requestID string) (HTTPRequest, error) {
	const q = `
		SELECT id, tunnel_id, method, path, query, headers, body,
		       status_code, resp_headers, resp_body, duration_ms, created_at
		FROM http_requests
		WHERE tunnel_id = $1 AND id = $2
	`
	var row HTTPRequest
	var query sql.NullString
	err := s.pool.QueryRow(ctx, q, tunnelID, requestID).Scan(
		&row.ID, &row.TunnelID, &row.Method, &row.Path, &query,
		&row.Headers, &row.Body, &row.StatusCode, &row.RespHeaders, &row.RespBody,
		&row.DurationMs, &row.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return HTTPRequest{}, ErrNotFound
		}
		return HTTPRequest{}, fmt.Errorf("get http request: %w", err)
	}
	row.Query = query.String
	return row, nil
}

func (s *HTTPRequestStore) Prune(ctx context.Context, tunnelID string, keep int) error {
	const q = `
		DELETE FROM http_requests
		WHERE tunnel_id = $1
		  AND (
		    created_at < now() - interval '24 hours'
		    OR id IN (
		      SELECT id FROM http_requests
		      WHERE tunnel_id = $1
		      ORDER BY created_at DESC
		      OFFSET $2
		    )
		  )
	`
	_, err := s.pool.Exec(ctx, q, tunnelID, keep)
	if err != nil {
		return fmt.Errorf("prune http requests: %w", err)
	}
	return nil
}

func (s *HTTPRequestStore) TunnelOwnedByUser(ctx context.Context, tunnelID, userID string) (bool, error) {
	const q = `SELECT 1 FROM tunnels WHERE id = $1 AND user_id = $2 LIMIT 1`
	var one int
	err := s.pool.QueryRow(ctx, q, tunnelID, userID).Scan(&one)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("check tunnel owner: %w", err)
	}
	return true, nil
}

func (s *HTTPRequestStore) scanMany(ctx context.Context, q string, args ...any) ([]HTTPRequest, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query http requests: %w", err)
	}
	defer rows.Close()

	var out []HTTPRequest
	for rows.Next() {
		var row HTTPRequest
		var query sql.NullString
		if err := rows.Scan(
			&row.ID, &row.TunnelID, &row.Method, &row.Path, &query,
			&row.Headers, &row.Body, &row.StatusCode, &row.RespHeaders, &row.RespBody,
			&row.DurationMs, &row.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan http request: %w", err)
		}
		row.Query = query.String
		out = append(out, row)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
