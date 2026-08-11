#!/usr/bin/env bash
# Пишет .env для Docker Compose на prod (не bash source).
# Экранирование: \ → \\, " → \", $ → $$ (иначе Compose съест $ в пароле).
# DATABASE_URL: пароль отдельно URL-encode.
set -euo pipefail

ENV_FILE="${1:?usage: vps-write-deploy-env.sh <outfile>}"

: >"$ENV_FILE"

write_kv_compose() {
  local key="$1"
  local value="$2"
  local escaped="$value"
  escaped="${escaped//\\/\\\\}"
  escaped="${escaped//\"/\\\"}"
  escaped="${escaped//\$/\$\$}"
  printf '%s="%s"\n' "$key" "$escaped" >>"$ENV_FILE"
}

: "${IMAGE_PREFIX:=forward}"
: "${IMAGE_TAG:?}"
: "${COMPOSE_PROJECT_NAME:?}"
: "${POSTGRES_USER:=forward}"
: "${POSTGRES_PASSWORD:?}"
: "${POSTGRES_DB:=forward}"
: "${JWT_SECRET:?}"
: "${BASE_DOMAIN:=frwrd.space}"
: "${WEB_PORT:?}"
: "${DATA_PATH:?}"

pass_enc="$(
  POSTGRES_PASSWORD="$POSTGRES_PASSWORD" python3 -c \
    'import os, urllib.parse; print(urllib.parse.quote(os.environ["POSTGRES_PASSWORD"], safe=""))'
)"
DATABASE_URL="postgres://${POSTGRES_USER}:${pass_enc}@postgres:5432/${POSTGRES_DB}?sslmode=disable"

write_kv_compose IMAGE_PREFIX "$IMAGE_PREFIX"
write_kv_compose IMAGE_TAG "$IMAGE_TAG"
write_kv_compose COMPOSE_PROJECT_NAME "$COMPOSE_PROJECT_NAME"
write_kv_compose POSTGRES_USER "$POSTGRES_USER"
write_kv_compose POSTGRES_PASSWORD "$POSTGRES_PASSWORD"
write_kv_compose POSTGRES_DB "$POSTGRES_DB"
write_kv_compose DATABASE_URL "$DATABASE_URL"
write_kv_compose JWT_SECRET "$JWT_SECRET"
write_kv_compose BASE_DOMAIN "$BASE_DOMAIN"
write_kv_compose WEB_PORT "$WEB_PORT"
write_kv_compose DATA_PATH "$DATA_PATH"
write_kv_compose SECURE_COOKIES "true"
write_kv_compose FORWARD_BIND "127.0.0.1"

chmod 600 "$ENV_FILE"
