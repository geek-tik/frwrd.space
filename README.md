# Forward

Сервис обратного туннелирования для разработчиков. Домен: **frwrd.space**.

Весь стек контейнеризован. Исключение — **хостовой nginx** (TLS, редиректы, маршрутизация `*.frwrd.space`).

**Продакшен:** https://frwrd.space · деплой: [deploy/SERVER_SETUP.md](./deploy/SERVER_SETUP.md)

Лицензия: [MIT](./LICENSE).

---

## Локальная разработка

```bash
cp .env.example .env
make up
```

| Сервис   | Порт  | Назначение               |
|----------|-------|--------------------------|
| server   | 8080  | REST API                 |
| server   | 8081  | Edge (туннели)           |
| server   | 8082  | Agent WebSocket          |
| web      | 3080* | Dashboard (`WEB_PORT`)   |
| postgres | —     | БД (только внутри compose) |

\* по умолчанию 3080; на сервере задайте свободный `WEB_PORT` и тот же порт в nginx

```bash
curl http://localhost:8080/health
open http://localhost:3080
```

### Туннель (локальный сервер)

**Сначала** запустите сервис на порту, **потом** агент:

```bash
# терминал 1 — ваш dev-сервер
python3 -m http.server 3000

# терминал 2 — токен из http://localhost:3080
make tunnel PORT=3000
```

Проверка без nginx:

```bash
curl -H "Host: <subdomain>.frwrd.space" http://localhost:8081/
```

Тест без сервиса на хосте (demo-app в compose):

```bash
docker compose --profile tunnel up -d demo-app
FORWARD_API_TOKEN=fwd_... FORWARD_LOCAL_HOST=demo-app \
  docker compose --profile tunnel run --rm agent http 3000
```

---

## Туннель к продакшену (с машины разработчика)

1. Зарегистрируйтесь на https://frwrd.space
2. Создайте API token в dashboard
3. **Запустите локальный сервис** на нужном порту
4. Запустите агент:

```bash
FORWARD_API_TOKEN=fwd_... \
FORWARD_SERVER_URL=wss://connect.frwrd.space/agent/connect \
FORWARD_DASHBOARD_URL=https://frwrd.space \
docker compose --profile tunnel run --rm agent http 3000
```

Публичный URL вида `https://<subdomain>.frwrd.space`. Запросы — в инспекторе на dashboard.

---

## Продакшен на сервере

Полная инструкция: [deploy/SERVER_SETUP.md](./deploy/SERVER_SETUP.md)

Кратко:

- Каталог `/var/www/forward`, пользователь `forward`, GitHub deploy key
- `docker compose up -d --build`
- nginx: bootstrap → certbot DNS → `host.conf.example`
- Порты API/edge/agent и dashboard только на `127.0.0.1`
- До публичного запуска: basic auth на dashboard и API (см. SERVER_SETUP)
- Редиректы: http→https, www→без www

Nginx проксирует:

| Хост | Бэкенд |
|------|--------|
| `frwrd.space` | web (`WEB_PORT`, localhost) |
| `api.frwrd.space` | server:8080 |
| `connect.frwrd.space` | server:8082 (WebSocket) |
| `*.frwrd.space` | server:8081 (edge) |

Обновление после `git pull` — раздел 8 в SERVER_SETUP.md.

### `.env` на продакшене

```bash
BASE_DOMAIN=frwrd.space
JWT_SECRET=<длинная случайная строка>
POSTGRES_PASSWORD=<не forward>
SECURE_COOKIES=true
FORWARD_BIND=127.0.0.1
WEB_PORT=<хостовой порт dashboard>
```

---

## Структура репозитория

```
cmd/server/     — API, edge, agent endpoint
cmd/forward/    — CLI-агент
cmd/migrate/    — миграции БД
internal/       — бизнес-логика
web/            — React dashboard
site/           — черновик статики (не в Docker-образе)
deploy/         — Docker, nginx, SERVER_SETUP.md
migrations/     — SQL
```

---

## Документация

| Файл | Назначение |
|------|------------|
| [docs/vision.md](./docs/vision.md) | Видение продукта (живой документ команды) |
| [README.md](./README.md) | Эксплуатация, quickstart |
| [deploy/SERVER_SETUP.md](./deploy/SERVER_SETUP.md) | Деплой и обновление сервера |
| [RESEARCH.md](./RESEARCH.md) | Исследование (архив, этап проектирования) |
| [MVP_PLAN.md](./MVP_PLAN.md) | План MVP (архив, этап проектирования) |

---

## Реализовано (MVP)

- Auth: регистрация, JWT, API tokens
- HTTP-туннели: WebSocket + yamux, `*.frwrd.space`
- Dashboard: туннели, инспектор запросов (SSE)
- Деплой: Docker + nginx + wildcard TLS

## Не в MVP (roadmap)

- Фиксированный поддомен, basic auth на туннель
- TCP-туннели
- Кастомные домены, команды, `forward.yml`
