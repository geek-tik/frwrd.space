# Деплой на сервер

Код как у сайта: **`/var/www/forward`**, пользователь **`forward`**. Стек — Docker Compose; снаружи только host nginx (80/443).

Порты API / edge / agent / dashboard — только `127.0.0.1`.

---

## 1. Пользователь и каталог (root)

```bash
sudo adduser --disabled-password --gecos "Forward" forward
sudo usermod -aG docker forward
sudo mkdir -p /var/www/forward
sudo chown forward:forward /var/www/forward
```

---

## 2. Deploy key и клон (пользователь forward)

```bash
sudo su - forward
ssh-keygen -t ed25519 -C "forward@frwrd.space" -f ~/.ssh/forward_deploy -N ""
cat ~/.ssh/forward_deploy.pub
```

Публичный ключ → GitHub → **Deploy keys** (read-only).

```bash
cat >> ~/.ssh/config <<'EOF'

Host github-forward
    HostName github.com
    User git
    IdentityFile ~/.ssh/forward_deploy
    IdentitiesOnly yes
EOF
chmod 700 ~/.ssh && chmod 600 ~/.ssh/config ~/.ssh/forward_deploy

git clone git@github-forward:<owner>/forward.git /var/www/forward
cd /var/www/forward
```

Обновление позже: `cd /var/www/forward && git pull && docker compose up -d --build`

---

## 3. Порты

Postgres из compose на хост **не** публикуется.

| Порт | Назначение | Bind |
|------|------------|------|
| 80, 443 | host nginx | публичный |
| 8080 | API | `127.0.0.1` |
| 8081 | Edge | `127.0.0.1` |
| 8082 | Agent WebSocket | `127.0.0.1` |
| `WEB_PORT` | Dashboard | `127.0.0.1` |

```bash
WEB_PORT=3080   # или другой свободный
for p in 80 443 8080 8081 8082 "$WEB_PORT"; do
  ss -tlnH "sport = :$p" 2>/dev/null | grep -q . \
    && echo "ЗАНЯТ  :$p" || echo "свободен :$p"
done
```

Тот же `WEB_PORT` — в `.env` и в `proxy_pass` nginx (блок dashboard).

---

## 4. Запуск приложения

```bash
cd /var/www/forward
cp .env.example .env
nano .env   # JWT_SECRET, POSTGRES_PASSWORD, BASE_DOMAIN,
            # FORWARD_BIND=127.0.0.1, WEB_PORT, SECURE_COOKIES=true
docker compose up -d --build

curl -s http://127.0.0.1:8080/health
curl -sI "http://127.0.0.1:${WEB_PORT:-3080}/" | head -3
```

---

## 5. Nginx и TLS (root)

Wildcard `*.frwrd.space` — только **DNS-challenge** (не `certbot --nginx`).

1. Временный HTTP-конфиг → nginx поднимается  
2. Certbot DNS → сертификат  
3. Финальный SSL-конфиг  

```bash
cp /var/www/forward/deploy/nginx/host.bootstrap.conf.example \
  /etc/nginx/sites-available/forward
# proxy_pass dashboard = WEB_PORT
nginx -t && systemctl reload nginx

certbot certonly --manual --preferred-challenges dns \
  -d frwrd.space -d '*.frwrd.space'
# TXT _acme-challenge.frwrd.space у DNS → Enter

cp /var/www/forward/deploy/nginx/host.conf.example \
  /etc/nginx/sites-available/forward
# снова сверьте WEB_PORT в proxy_pass
nginx -t && systemctl reload nginx

curl -sI https://frwrd.space | head -3
```

Включите сайт, если ещё не симлинк:

```bash
ln -sf /etc/nginx/sites-available/forward /etc/nginx/sites-enabled/forward
```

---

## 6. Pre-launch: basic auth

Пока регистрации не открыты публично — basic auth на **`frwrd.space` и `api.frwrd.space`**.  
Иначе UI закрыт, а `POST /api/v1/auth/register` на `api.` остаётся открытым.

`connect.` и `*.` (edge) — **без** basic auth (агент + публичные туннели).

```bash
sudo apt-get install -y apache2-utils
sudo mkdir -p /etc/nginx/htpasswd
sudo htpasswd -c /etc/nginx/htpasswd/forward team
sudo chmod 640 /etc/nginx/htpasswd/forward
sudo chown root:www-data /etc/nginx/htpasswd/forward
```

В обоих `server` (`frwrd.space`, `api.frwrd.space`) раскомментируйте (есть в `host.conf.example`):

```nginx
auth_basic           "Forward private";
auth_basic_user_file /etc/nginx/htpasswd/forward;
```

```bash
nginx -t && systemctl reload nginx
```

Снять перед публичным запуском: убрать `auth_basic*`, reload.

---

## 7. Обновление

```bash
sudo -u forward bash -c \
  'cd /var/www/forward && git pull && docker compose up -d --build'
# если меняли deploy/nginx/ — скопируйте конфиг снова, сохраните auth_basic
nginx -t && systemctl reload nginx
```

---

## 8. Туннель с ноутбука

При basic auth: откройте https://frwrd.space в браузере → регистрация → API token. Агенту htpasswd не нужен.

```bash
FORWARD_API_TOKEN=fwd_... \
FORWARD_SERVER_URL=wss://connect.frwrd.space/agent/connect \
FORWARD_DASHBOARD_URL=https://frwrd.space \
docker compose --profile tunnel run --rm agent http 3000
```

---

| Что | Где |
|-----|-----|
| Код | `/var/www/forward` |
| Deploy key | `~forward/.ssh/forward_deploy` |
| htpasswd | `/etc/nginx/htpasswd/forward` |
| nginx site | `/etc/nginx/sites-available/forward` |
