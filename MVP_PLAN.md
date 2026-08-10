# Forward — план реализации MVP на Go

> **Статус:** MVP реализован (июнь 2026), продакшен: **frwrd.space**.  
> Актуальная документация: [README.md](./README.md), [deploy/SERVER_SETUP.md](./deploy/SERVER_SETUP.md).  
> Этот файл — технический план на этапе проектирования.

Документ описывает техническую реализацию MVP из [RESEARCH.md](./RESEARCH.md):

- CLI-агент (один бинарник, в Docker)
- HTTP/HTTPS туннели
- Регистрация + API token
- Случайные поддомены `*.frwrd.space`
- Веб-UI: список туннелей + HTTP request inspector
- Авто-HTTPS (nginx + Let's Encrypt)

**Оценка:** 6–8 недель, 1–2 разработчика.

---

## 1. Архитектура

### 1.1 Компоненты

```
                         ┌─────────────────────────────────────┐
                         │           PostgreSQL                │
                         └──────────────────┬──────────────────┘
                                            │
┌──────────┐   HTTPS    ┌───────────────────▼───────────────────┐
│ Browser  │───────────►│              Edge Server              │
│ (public) │            │  • TLS termination (*.forward.ru)     │
└──────────┘            │  • HTTP routing by subdomain          │
                        │  • Request capture → inspector        │
                        └───────────────┬───────────────────────┘
                                        │ yamux stream
                        ┌───────────────▼───────────────────────┐
                        │           Control Plane               │
                        │  • Auth (JWT / API tokens)            │
                        │  • Tunnel registry (in-memory+DB)     │
                        │  • Agent WebSocket/gRPC endpoint    │
                        │  • REST API для dashboard           │
                        └───────────────┬───────────────────────┘
                                        │ outbound WSS (443)
                        ┌───────────────▼───────────────────────┐
                        │         forward CLI (agent)           │
                        │  • Подключение к control plane      │
                        │  • Проксирование на localhost:port  │
                        └───────────────┬───────────────────────┘
                                        │ localhost
                        ┌───────────────▼───────────────────────┐
                        │      dev server :3000                 │
                        └───────────────────────────────────────┘

┌──────────┐   HTTPS    ┌───────────────────────────────────────┐
│ Dashboard│───────────►│  Web UI (React) + REST API            │
│ (browser)│◄─── SSE ───│  • Список туннелей                    │
└──────────┘            │  • HTTP inspector (live + history)    │
                        └───────────────────────────────────────┘
```

### 1.2 Решения для MVP

| Решение | Выбор | Почему |
|---------|-------|--------|
| Язык бэкенда | **Go 1.22+** | Один бинарник агента, goroutines, стандартная экосистема для сетевых прокси |
| Мультиплексирование | **yamux** | Проверен в frp/inlets, несколько HTTP-запросов по одному TCP-соединению агента |
| Транспорт агент ↔ сервер | **WebSocket over TLS** | Проходит через корпоративные прокси и NAT (порт 443) |
| БД | **PostgreSQL** | Пользователи, токены, история запросов |
| Кэш/реестр туннелей | **in-memory + Redis** (опционально) | Активные сессии — в памяти edge; Redis — если >1 инстанса edge |
| TLS | **cert-manager + Let's Encrypt** | Wildcard `*.forward.ru` |
| Веб-UI | **React + Vite** | Отдельное SPA, общается с REST API |
| Монорепо | **go workspaces** | `cmd/`, `internal/`, `pkg/`, `web/` |

### 1.3 Что сознательно НЕ делаем в MVP

- TCP- и TLS-туннели (только HTTP/HTTPS; см. v1.1)
- Фиксированные поддомены
- Кастомные домены
- Команды / организации
- Rate limiting (кроме базового лимита туннелей на пользователя)
- Kubernetes operator
- Replay запросов (только просмотр; replay — v1.1)

---

## 2. Структура репозитория

```
forward/
├── cmd/
│   ├── forward/          # CLI-агент (пользовательский бинарник)
│   ├── server/           # Control plane + edge (один процесс для MVP)
│   └── migrate/          # Миграции БД
├── internal/
│   ├── agent/            # Логика агента: connect, proxy, reconnect
│   ├── auth/             # JWT, API tokens, middleware
│   ├── edge/             # Публичный HTTP handler, subdomain routing
│   ├── inspector/        # Захват и хранение HTTP req/res
│   ├── protocol/         # Wire format, messages, yamux session
│   ├── registry/         # Реестр активных туннелей
│   ├── subdomain/        # Генерация случайных поддоменов
│   └── store/            # PostgreSQL repositories
├── pkg/
│   └── forward/          # Публичные типы (если понадобится SDK)
├── web/                  # React dashboard
│   ├── src/
│   └── package.json
├── migrations/           # SQL миграции (goose)
├── deploy/
│   ├── docker-compose.yml
│   └── k8s/              # Позже
├── scripts/
│   └── install.sh
├── go.work
├── go.mod
└── Makefile
```

---

## 3. Протокол туннеля

### 3.1 Жизненный цикл

```
1. Пользователь: forward auth <api_token>
   → токен сохраняется в ~/.forward/config.yml

2. Пользователь: forward http 3000
   → агент открывает WSS к server: /agent/connect
   → аутентификация: Authorization: Bearer <api_token>

3. Агент отправляет RegisterTunnel:
   { "local_port": 3000, "protocol": "http" }

4. Сервер:
   → генерирует subdomain (например, "k7m2xq")
   → регистрирует в registry: k7m2xq → session_id
   → отвечает: { "url": "https://k7m2xq.forward.ru", "tunnel_id": "..." }

5. Внешний запрос GET https://k7m2xq.forward.ru/api/users
   → edge извлекает subdomain → находит session в registry
   → открывает yamux stream на агенте
   → агент проксирует на http://127.0.0.1:3000/api/users
   → ответ идёт обратно

6. Агент отключается → туннель удаляется из registry
```

### 3.2 Формат сообщений (JSON over WebSocket для control, yamux для data)

**Control messages (WebSocket text frames):**

```json
// Agent → Server
{ "type": "register", "local_addr": "127.0.0.1:3000", "protocol": "http" }
{ "type": "ping" }

// Server → Agent
{ "type": "registered", "tunnel_id": "uuid", "subdomain": "k7m2xq", "public_url": "https://k7m2xq.forward.ru" }
{ "type": "error", "message": "..." }
{ "type": "pong" }
```

**Data plane:** после `registered` сервер и агент используют yamux session поверх того же WebSocket (binary frames) или отдельного TCP channel. Для MVP — **yamux поверх WebSocket binary**.

### 3.3 HTTP-проксирование на агенте

Агент получает yamux stream, читает HTTP request (или формирует из метаданных edge), проксирует через `httputil.ReverseProxy`:

```go
// Упрощённо
proxy := httputil.NewSingleHostReverseProxy(
    &url.URL{Scheme: "http", Host: "127.0.0.1:3000"},
)
// Модификация Host, X-Forwarded-For, X-Forwarded-Proto
```

Edge передаёт оригинальные заголовки и тело; агент не парсит subdomain — это делает edge.

---

## 4. База данных

### 4.1 Схема

```sql
-- migrations/001_init.sql

CREATE TABLE users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE api_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT NOT NULL UNIQUE,  -- храним SHA-256, не plaintext
    name        TEXT NOT NULL DEFAULT 'default',
    last_used_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tunnels (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id),
    subdomain   TEXT NOT NULL,
    local_port  INT NOT NULL,
    protocol    TEXT NOT NULL DEFAULT 'http',
    status      TEXT NOT NULL DEFAULT 'active',  -- active, closed
    started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    closed_at   TIMESTAMPTZ
);

CREATE INDEX idx_tunnels_user_id ON tunnels(user_id);
CREATE INDEX idx_tunnels_subdomain ON tunnels(subdomain);

CREATE TABLE http_requests (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tunnel_id   UUID NOT NULL REFERENCES tunnels(id) ON DELETE CASCADE,
    method      TEXT NOT NULL,
    path        TEXT NOT NULL,
    query       TEXT,
    headers     JSONB NOT NULL DEFAULT '{}',
    body        BYTEA,               -- лимит 1 MB в MVP
    status_code INT,
    resp_headers JSONB,
    resp_body   BYTEA,               -- лимит 1 MB в MVP
    duration_ms INT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_http_requests_tunnel_id ON http_requests(tunnel_id);
CREATE INDEX idx_http_requests_created_at ON http_requests(created_at);
```

### 4.2 Лимиты MVP

| Параметр | Значение |
|----------|----------|
| Активных туннелей на пользователя | 3 |
| История запросов на туннель | 100 последних |
| Размер body в inspector | 1 MB |
| Длина subdomain | 6 символов `[a-z0-9]` |
| TTL записи в inspector | 24 часа |

---

## 5. API

### 5.1 Публичные endpoint'ы (auth)

```
POST   /api/v1/auth/register     { email, password }
POST   /api/v1/auth/login        { email, password } → { access_token, refresh_token }
POST   /api/v1/auth/refresh      { refresh_token }
```

### 5.2 Управление токенами (JWT auth)

```
GET    /api/v1/tokens
POST   /api/v1/tokens            { name } → { token }  // plaintext только при создании
DELETE /api/v1/tokens/:id
```

### 5.3 Туннели (JWT auth, для dashboard)

```
GET    /api/v1/tunnels           → список (активные + недавние)
GET    /api/v1/tunnels/:id
DELETE /api/v1/tunnels/:id       → принудительное закрытие
```

### 5.4 Inspector (JWT auth)

```
GET    /api/v1/tunnels/:id/requests         → список с пагинацией
GET    /api/v1/tunnels/:id/requests/:req_id → детали
GET    /api/v1/tunnels/:id/requests/stream  → SSE, live-обновления
```

### 5.5 Agent endpoint (API token auth)

```
WSS    /agent/connect             Authorization: Bearer <api_token>
```

### 5.6 Edge (публичный, без auth)

```
*      https://{subdomain}.forward.ru/*  → проксирование через туннель
```

---

## 6. CLI-агент

### 6.1 Команды

```bash
forward auth <token>          # Сохранить API token
forward http <port>           # Запустить HTTP туннель
forward status                # Текущий туннель (если есть)
forward version               # Версия
forward config check          # Проверить конфиг
```

### 6.2 Конфиг `~/.forward/config.yml`

```yaml
api_token: "fwd_xxxxxxxxxxxx"
server_url: "https://connect.forward.ru"  # default
```

### 6.3 UX при запуске

```
$ forward http 3000

  Туннель запущен

  Публичный URL   https://k7m2xq.forward.ru
  Локальный       http://127.0.0.1:3000
  Инспектор       https://dashboard.forward.ru/tunnels/abc-123

  Нажмите Ctrl+C для остановки
```

### 6.4 Зависимости CLI

| Пакет | Назначение |
|-------|------------|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/hashicorp/yamux` | Stream multiplexing |
| `github.com/gorilla/websocket` | WSS transport |
| `gopkg.in/yaml.v3` | Конфиг |
| `github.com/fatih/color` | Цветной вывод (опционально) |

### 6.5 Reconnect

- Exponential backoff: 1s → 2s → 4s → ... → 30s max
- При reconnect — новый subdomain (MVP: старый освобождается)
- Graceful shutdown по SIGINT/SIGTERM

---

## 7. Edge Server и HTTPS

### 7.1 Subdomain routing

```go
// Псевдокод middleware
func SubdomainRouter(registry *registry.Registry) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        host := r.Host // k7m2xq.forward.ru
        subdomain := extractSubdomain(host) // k7m2xq

        session, ok := registry.Get(subdomain)
        if !ok {
            http.Error(w, "Туннель не найден", http.StatusNotFound)
            return
        }

        proxyToTunnel(w, r, session)
    })
}
```

### 7.2 TLS

- Wildcard-сертификат `*.forward.ru` + `forward.ru`
- **cert-manager** в Kubernetes или **Caddy** / **lego** для автоматического обновления
- Edge принимает HTTPS, общается с агентом по WSS (тоже TLS)
- Заголовок `X-Forwarded-Proto: https` при проксировании на localhost

### 7.3 Заголовки при проксировании

Агент добавляет к запросу на localhost:

```
X-Forwarded-For: <client-ip>
X-Forwarded-Proto: https
X-Forwarded-Host: k7m2xq.forward.ru
```

---

## 8. HTTP Request Inspector

### 8.1 Захват

Edge (или отдельный middleware) при каждом запросе:

1. Читает request body (с лимитом 1 MB, tee reader)
2. Проксирует через туннель
3. Читает response body (с лимитом 1 MB)
4. Асинхронно сохраняет в `http_requests`
5. Публикует событие в SSE-подписчиков dashboard

### 8.2 Live-обновления (SSE)

```
GET /api/v1/tunnels/:id/requests/stream
Authorization: Bearer <jwt>

event: request
data: {"id":"...","method":"POST","path":"/webhook","status_code":200,...}
```

Dashboard подписывается при открытии страницы туннеля.

### 8.3 UI inspector (React)

- Таблица запросов: метод, путь, статус, время, duration
- Клик → панель деталей: headers, body (pretty JSON), response
- Автоскролл при новых запросах
- Фильтр по методу / статусу (простой, client-side)

---

## 9. Веб-UI (Dashboard)

### 9.1 Страницы MVP

| Страница | Функции |
|----------|---------|
| `/login` | Email + password |
| `/register` | Регистрация |
| `/` | Список активных туннелей, кнопка «Создать токен» |
| `/tokens` | CRUD API tokens, copy-to-clipboard |
| `/tunnels/:id` | Детали туннеля + live inspector |
| `/onboarding` | Wizard: установка CLI → auth → первая команда |

### 9.2 Onboarding wizard

```
Шаг 1: Зарегистрируйтесь ✓
Шаг 2: Создайте API token    [Создать] [Скопировать]
Шаг 3: Установите CLI        curl -sSL https://forward.ru/install | sh
Шаг 4: Авторизуйтесь         forward auth fwd_xxxxx
Шаг 5: Запустите туннель     forward http 3000
```

### 9.3 Стек фронтенда

| Пакет | Назначение |
|-------|------------|
| React 18 + Vite | SPA |
| TanStack Query | Кэш API, polling/SSE |
| React Router | Роутинг |
| Tailwind CSS | Стили |
| shadcn/ui | Компоненты |

---

## 10. Безопасность

### 10.1 Аутентификация

- **Dashboard:** JWT (access 15 min, refresh 7 days), httpOnly cookie для refresh
- **Agent:** API token `fwd_` + 32 random bytes, хранится как SHA-256 в БД
- Пароли: bcrypt cost 12

### 10.2 Защита edge

- Только зарегистрированные subdomain'ы принимают трафик
- Нет произвольного proxy (защита от open proxy)
- Request body size limit: 10 MB (edge), 1 MB (inspector storage)
- Timeouts: read 30s, write 30s, idle 120s

### 10.3 Что отложить

- IP whitelist
- Basic auth на туннель
- Rate limiting per IP
- mTLS для агента

---

## 11. Деплой

### 11.1 MVP-инфраструктура

```
┌─────────────────────────────────────────────┐
│  VPS / Cloud (РФ)                           │
│                                             │
│  ┌─────────┐  ┌──────────┐  ┌───────────┐  │
│  │ Caddy   │  │ server   │  │ PostgreSQL│  │
│  │ (TLS)   │──│ (Go)     │──│           │  │
│  └─────────┘  └──────────┘  └───────────┘  │
│                                             │
│  DNS: *.forward.ru → VPS                    │
│       dashboard.forward.ru → VPS            │
│       connect.forward.ru → VPS              │
└─────────────────────────────────────────────┘
```

### 11.2 docker-compose (dev/staging)

```yaml
services:
  postgres:
    image: postgres:16
  server:
    build: .
    ports: ["8080:8080", "8443:8443"]
    environment:
      DATABASE_URL: postgres://...
      BASE_DOMAIN: forward.ru
  web:
    build: ./web
    ports: ["3000:80"]
```

### 11.3 CI/CD

- GitHub Actions / GitLab CI
- `go test ./...` + lint (`golangci-lint`)
- Сборка бинарников: `linux/amd64`, `linux/arm64`, `darwin/arm64`, `darwin/amd64`
- `install.sh` скачивает нужный бинарник с releases

---

## 12. Этапы реализации

### Этап 0 — Фундамент (неделя 1)

- [ ] Инициализация монорепо (`go mod`, `go.work`, Makefile)
- [ ] PostgreSQL + миграции (goose)
- [ ] Модели и repository layer
- [ ] `cmd/server` — HTTP сервер с health check
- [ ] docker-compose для локальной разработки

**Критерий готовности:** `make dev` поднимает postgres + server, `GET /health` → 200.

---

### Этап 1 — Auth (неделя 1–2)

- [ ] Register / login / JWT
- [ ] API tokens CRUD
- [ ] Middleware auth (JWT для API, token для agent)
- [ ] Unit-тесты auth

**Критерий готовности:** можно создать пользователя, получить JWT и API token через curl.

---

### Этап 2 — Протокол туннеля (неделя 2–3)

- [ ] WebSocket endpoint `/agent/connect`
- [ ] yamux session поверх WS
- [ ] Registry (in-memory): subdomain → session
- [ ] Генератор subdomain (6 chars, проверка коллизий)
- [ ] `cmd/forward` — cobra CLI, `auth`, `http`
- [ ] Агент: connect → register → proxy localhost

**Критерий готовности:** `forward http 3000` + `curl http://localhost:8080` (без TLS, dev mode) — запрос доходит до локального сервера.

---

### Этап 3 — Edge + HTTPS (неделя 3–4)

- [ ] Subdomain routing middleware
- [ ] TLS termination (Caddy или встроенный)
- [ ] DNS `*.forward.ru`
- [ ] X-Forwarded-* заголовки
- [ ] Запись туннелей в БД (start/close)
- [ ] Reconnect + graceful shutdown агента

**Критерий готовности:** `curl https://k7m2xq.forward.ru` с телефона (не в локальной сети) открывает localhost.

---

### Этап 4 — Inspector (неделя 4–5)

- [ ] Middleware захвата req/res на edge
- [ ] Асинхронная запись в `http_requests`
- [ ] API: список, детали, SSE stream
- [ ] Лимиты body, ротация (100 записей / 24h)
- [ ] Вывод URL инспектора в CLI

**Критерий готовности:** запросы видны в API сразу после прохождения через туннель.

---

### Этап 5 — Dashboard (неделя 5–6)

- [ ] React scaffold (Vite + Tailwind + shadcn)
- [ ] Login / register
- [ ] Страница токенов
- [ ] Список туннелей (polling 5s)
- [ ] Страница туннеля + live inspector (SSE)
- [ ] Onboarding wizard

**Критерий готовности:** полный flow без CLI-команд кроме `forward auth` и `forward http`.

---

### Этап 6 — Полировка и релиз (неделя 6–8)

- [ ] `install.sh` + GitHub Releases (4 платформы)
- [ ] Русские сообщения об ошибках в CLI
- [ ] Логирование (slog, structured JSON)
- [ ] Метрики: активные туннели, requests/sec (Prometheus, базово)
- [ ] Нагрузочный тест: 100 concurrent requests через 1 туннель
- [ ] README, docs.forward.ru (базовая)
- [ ] Staging deploy

**Критерий готовности:** новый пользователь проходит onboarding за < 2 минут.

---

## 13. Ключевые Go-пакеты (сводка)

| Область | Пакет |
|---------|-------|
| CLI | `spf13/cobra` |
| HTTP router | `go-chi/chi/v5` |
| WebSocket | `coder/websocket` |
| Multiplexing | `hashicorp/yamux` |
| PostgreSQL | `jackc/pgx/v5` |
| Миграции | `pressly/goose/v3` |
| JWT | `golang-jwt/jwt/v5` |
| Config | `knadh/koanf` или yaml |
| UUID | `google/uuid` |
| Logging | `log/slog` (stdlib) |
| Тесты | `stretchr/testify` |

---

## 14. Риски и митигация

| Риск | Вероятность | Митигация |
|------|-------------|-----------|
| WebSocket блокируется корп. прокси | Средняя | Fallback на HTTP/2 CONNECT (v1.1); MVP — документировать |
| Высокая latency через yamux | Низкая | Бенчмарки на этапе 2; профилирование pprof |
| Wildcard TLS сложно настроить | Средняя | Caddy автоматизирует; dev — mkcert |
| Утечка памяти в inspector | Средняя | Жёсткие лимиты body, ротация записей |
| Subdomain collision | Низкая | 36^6 ≈ 2 млрд комбинаций; проверка в БД |
| Один инстанс — SPOF | Высокая (MVP) | Приемлемо для MVP; Redis + multi-edge в v1.1 |

---

## 15. Критерии приёмки MVP

1. `curl -sSL https://forward.ru/install | sh` устанавливает CLI на Linux/macOS
2. `forward auth <token>` + `forward http 3000` выдаёт рабочий `https://*.forward.ru` URL
3. HTTPS работает без предупреждений браузера
4. Запросы к туннелю видны в dashboard в реальном времени (< 1s задержка)
5. Туннель закрывается при Ctrl+C, subdomain освобождается
6. Регистрация, логин, создание API token — через веб-UI
7. Документация на русском: quickstart на 1 страницу

---

## 16. Статус реализации

MVP выполнен. Отличия от плана:

- Домен: `frwrd.space` (не `forward.ru`)
- Всё в Docker (без `curl install | sh`)
- TLS на хостовом nginx, wildcard через DNS-challenge
- `WEB_PORT` настраивается через `.env`

Дальнейшие шаги — roadmap в [README.md](./README.md).

---

*Документ создан: 29 июня 2026*
