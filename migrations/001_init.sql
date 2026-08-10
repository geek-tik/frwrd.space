-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_tokens (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT NOT NULL UNIQUE,
    name         TEXT NOT NULL DEFAULT 'default',
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tunnels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    subdomain   TEXT NOT NULL,
    local_port  INT NOT NULL,
    protocol    TEXT NOT NULL DEFAULT 'http',
    status      TEXT NOT NULL DEFAULT 'active',
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at   TIMESTAMPTZ
);

CREATE INDEX idx_tunnels_user_id ON tunnels(user_id);
CREATE INDEX idx_tunnels_subdomain ON tunnels(subdomain);

CREATE TABLE http_requests (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tunnel_id    UUID NOT NULL REFERENCES tunnels(id) ON DELETE CASCADE,
    method       TEXT NOT NULL,
    path         TEXT NOT NULL,
    query        TEXT,
    headers      JSONB NOT NULL DEFAULT '{}',
    body         BYTEA,
    status_code  INT,
    resp_headers JSONB,
    resp_body    BYTEA,
    duration_ms  INT,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_http_requests_tunnel_id ON http_requests(tunnel_id);
CREATE INDEX idx_http_requests_created_at ON http_requests(created_at);

-- +goose Down
DROP TABLE IF EXISTS http_requests;
DROP TABLE IF EXISTS tunnels;
DROP TABLE IF EXISTS api_tokens;
DROP TABLE IF EXISTS users;
