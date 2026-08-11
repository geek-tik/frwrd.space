# Деплой на сервер

Образы собираются в GitHub Actions, на сервер копируются готовые артефакты и файлы compose. На сервере: `docker load` и `docker compose up -d` — без сборки и без git.

| | |
|--|--|
| Пользователь | `forward-prod` |
| Каталог приложения | `SERVER_PATH` (например `/srv/forward/prod`) |
| Каталог данных | `DATA_PATH` (например `/srv/forward/prod_data`) |
| `COMPOSE_PROJECT_NAME` | `forward-prod` |

Порты 8080, 8081, 8082 и `WEB_PORT` слушают только `127.0.0.1`. Снаружи — host nginx на 80/443.

Локальный запуск с `--build`: [README](../README.md).

---

## 1. Пользователь и каталоги (root)

```bash
sudo adduser --disabled-password --gecos "Forward" forward-prod
sudo usermod -aG docker forward-prod
sudo mkdir -p /srv/forward/prod /srv/forward/prod_data
sudo chown -R forward-prod:forward-prod /srv/forward
```

На сервере: Docker, Compose plugin, rsync. Фактические пути задайте в Variables (`SERVER_PATH`, `DATA_PATH`) и используйте их же в командах ниже.

---

## 2. SSH для выкладки

Нужен SSH-доступ пользователя `forward-prod` с runner (секрет `SSH_PRIVATE_KEY`). Репозиторий на сервер не клонируется.

```bash
ssh-keygen -t ed25519 -C "forward-prod-deploy" -f ./forward_deploy_prod -N ""
cat ./forward_deploy_prod.pub
```

На сервере (root):

```bash
sudo mkdir -p /home/forward-prod/.ssh
sudo tee -a /home/forward-prod/.ssh/authorized_keys <<<"ssh-ed25519 AAAA… "
sudo chown -R forward-prod:forward-prod /home/forward-prod/.ssh
sudo chmod 700 /home/forward-prod/.ssh
sudo chmod 600 /home/forward-prod/.ssh/authorized_keys
```

Проверка:

```bash
ssh -o IdentitiesOnly=yes -i ./forward_deploy_prod forward-prod@<SERVER_HOST> \
  'whoami && groups && ls -la "$HOME" && ls -la <SERVER_PATH>/..'
```

Приватный ключ — в GitHub Environment `prod`, secret `SSH_PRIVATE_KEY`. В git не коммитить.

---

## 3. Порты

| Порт | Назначение | Bind |
|------|------------|------|
| 80, 443 | host nginx | `0.0.0.0` |
| 8080 | API | `127.0.0.1` |
| 8081 | Edge | `127.0.0.1` |
| 8082 | Agent WebSocket | `127.0.0.1` |
| `WEB_PORT` | Dashboard | `127.0.0.1` |

Postgres на хост не публикуется. `WEB_PORT` и `proxy_pass` в nginx должны совпадать. Порты 8080–8082 и `WEB_PORT` снаружи не открывать.

```bash
WEB_PORT=3131
for p in 80 443 8080 8081 8082 "$WEB_PORT"; do
  ss -tlnH "sport = :$p" 2>/dev/null | grep -q . \
    && echo "ЗАНЯТ  :$p" || echo "свободен :$p"
done
```

---

## 4. GitHub Environment `prod`

**Secrets:** `SSH_PRIVATE_KEY`, `POSTGRES_PASSWORD`, `JWT_SECRET`

**Variables:**

| Name | Значение |
|------|----------|
| `SERVER_HOST` | хост сервера |
| `SERVER_USER` | `forward-prod` |
| `SERVER_PATH` | каталог приложения, например `/srv/forward/prod` |
| `DATA_PATH` | каталог данных, например `/srv/forward/prod_data` |
| `COMPOSE_PROJECT_NAME` | `forward-prod` |
| `WEB_PORT` | порт dashboard на localhost |
| `BASE_DOMAIN` | `frwrd.space` |

`.env` на сервере пишет `scripts/vps-write-deploy-env.sh`:

- `SECURE_COOKIES=true`, `FORWARD_BIND=127.0.0.1` — константы prod, не Variables;
- `POSTGRES_PASSWORD` / `JWT_SECRET`: в файле экранируются `\`, `"`, `$` → `$$` (требование Compose);
- `DATABASE_URL` собирается в скрипте, пароль в URL — percent-encode.

Пароль в Secret храните как есть (сырой). Не вставляйте уже экранированную строку вручную в `.env`.

Workflow: сборка образов → передача на сервер → `docker load` → скрипт `.env` → `compose up -d`.

---

## 5. Nginx и TLS (root)

Для `*.frwrd.space` нужен DNS-challenge:

```bash
certbot certonly --manual --preferred-challenges dns \
  -d frwrd.space -d '*.frwrd.space'
```

Примеры: `deploy/nginx/host.bootstrap.conf.example` (до сертификата), `deploy/nginx/host.conf.example` (после). В `proxy_pass` dashboard указать `WEB_PORT`.

```bash
cp /path/to/host.conf.example /etc/nginx/sites-available/forward
ln -sf /etc/nginx/sites-available/forward /etc/nginx/sites-enabled/forward
nginx -t && systemctl reload nginx
```

Конфиг nginx на сервер копируется отдельно от compose-выкладки.

---

## 6. Ограничение регистраций (basic auth)

Пока регистрация не открыта: basic auth на `frwrd.space` и `api.frwrd.space`. На `connect.` и `*.` не включать.

```bash
sudo apt-get install -y apache2-utils
sudo mkdir -p /etc/nginx/htpasswd
sudo htpasswd -c /etc/nginx/htpasswd/forward team
sudo chmod 640 /etc/nginx/htpasswd/forward
sudo chown root:www-data /etc/nginx/htpasswd/forward
```

В `server` для dashboard и API (см. `host.conf.example`):

```nginx
auth_basic           "Forward private";
auth_basic_user_file /etc/nginx/htpasswd/forward;
```

```bash
nginx -t && systemctl reload nginx
```

---

## 7. Обновление

Через GitHub Actions (merge в `main` или ручной запуск workflow). На сервере образы не собирать и git не использовать.

---

## 8. Агент с рабочей машины

```bash
FORWARD_API_TOKEN=fwd_... \
FORWARD_SERVER_URL=wss://connect.frwrd.space/agent/connect \
FORWARD_DASHBOARD_URL=https://frwrd.space \
docker compose --profile tunnel run --rm agent http 3000
```

При basic auth на dashboard сначала войти в браузере, создать token. Агенту basic auth не передаётся.

---

| | |
|--|--|
| Приложение | `SERVER_PATH` |
| Данные | `DATA_PATH` |
| SSH CI | `/home/forward-prod/.ssh/authorized_keys` |
| htpasswd | `/etc/nginx/htpasswd/forward` |
| nginx | `/etc/nginx/sites-available/forward` |
