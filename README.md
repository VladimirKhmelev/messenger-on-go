# Wisply

[![CI](https://github.com/VladimirKhmelev/messenger-on-go/actions/workflows/ci.yml/badge.svg)](https://github.com/VladimirKhmelev/messenger-on-go/actions/workflows/ci.yml)
[![Live](https://img.shields.io/badge/live-wisply.site-blue)](https://wisply.site)
[![Swagger](https://img.shields.io/badge/API-Swagger-85EA2D)](https://vladimirkhmelev.github.io/messenger-on-go/swagger/)
[![Go](https://img.shields.io/badge/Go-1.26-00ADD8)](https://go.dev)

Real-time чат с E2E-шифрованием сообщений, реализованный как monorepo из 4
независимых микросервисов

## Архитектура

```
                        ┌──────────────┐
   браузер ── HTTPS ──▶ │    nginx     │
                        └──────┬───────┘
                               │
        ┌───────────────┬─────┴────────┬───────────────┐
        ▼               ▼              ▼               │
 ┌─────────────┐ ┌─────────────┐ ┌─────────────┐       │
 │auth-service │ │chat-service │ │ ws-gateway  │◀──────┘ WS
 │  + Postgres │ │ + Postgres  │ │  (× 2 инст.)│
 │  + Redis    │ │ + Redis     │ └──────┬──────┘
 └──────┬──────┘ └──────┬──────┘        │
        │               │               │
        └───────────────┴───────────────┘
                         │ NATS JetStream / core pub-sub
                         ▼
              ┌────────────────────┐
              │ notification-worker │
              └────────────────────┘
```

| Сервис | Ответственность | Хранилище |
|---|---|---|
| `auth-service` | регистрация, логин, JWT, OAuth (GitHub), RSA-ключи для E2E | свой Postgres + Redis |
| `chat-service` | чаты, сообщения (зашифрованы на клиенте), presence, typing | свой Postgres + Redis |
| `ws-gateway` | держит WebSocket-соединения, офлайн-валидация JWT | без своей БД |
| `notification-worker` | слушает события, решает кому послать push-уведомление | без своей БД |

Сервисы не лезут в чужие таблицы напрямую — общаются через gRPC или события в
NATS.

## Ключевые особенности

- **End-to-end шифрование** — RSA-ключевая пара генерируется в браузере (Web
  Crypto API), приватный ключ хранится только на устройстве (обёрнутым
  паролем пользователя). Ключ каждого чата — отдельный AES-256-GCM ключ,
  зашифрованный RSA-публичным ключом каждого участника. Сервер физически не
  может прочитать содержимое сообщений.
- **Presence и typing-индикатор** — статус "в сети" привязан к реальной
  видимости вкладки/фокусу окна (`visibilitychange` + `blur`/`focus`), а не
  просто к открытому WS-соединению.
- **GitHub OAuth** — вход через GitHub с полноценной E2E-ключевой парой
  (не урезанный аккаунт без шифрования).
- **Realtime без потери сообщений** — переподключение с backoff, очередь
  исходящих сообщений на время разрыва, мёрж истории вместо перезаписи при
  гонке между `get_history` и live-сообщениями.

## Стек

```
              ┌─────────────────────────────────┐
   frontend   │  vanilla JS · Web Crypto API    │
              │  (RSA-OAEP + AES-256-GCM)       │
              ├─────────────────────────────────┤
   edge       │  nginx  +  Let's Encrypt        │
              ├─────────────────────────────────┤
   transport  │  gRPC + protobuf  ·  WebSocket  │
              ├─────────────────────────────────┤
   services   │  Go 1.26                        │
              ├─────────────────────────────────┤
   auth       │  JWT (golang-jwt)  +  bcrypt    │
              │  OAuth2 (GitHub)                │
              ├─────────────────────────────────┤
   messaging  │  NATS JetStream (nats.go)       │
              ├─────────────────────────────────┤
   storage    │  Postgres (pgx)  ·  Redis       │
              ├─────────────────────────────────┤
   runtime    │  Docker Compose                 │
              └─────────────────────────────────┘
```

## Структура репозитория

```
docker-compose.yml             все 4 сервиса + Postgres×2 + Redis + NATS + nginx
Makefile                       proto/up/down/unit/integration/lint/ci

services/
  auth-service/                 регистрация, логин, JWT, OAuth, RSA-ключи
    cmd/server/main.go            точка входа, wiring зависимостей
    internal/
      domain/                     доменные сущности + сентинел-ошибки
      repository/                 Postgres-репозиторий пользователей
      service/                    бизнес-логика (Register, Login, ChangePassword, ...)
      transport/
        grpc/                     gRPC-сервер + auth-интерцептор
        avatar/                   HTTP-хендлер загрузки/отдачи аватара (не protobuf JSON)
      oauth/                      GitHub OAuth-клиент
      mail/                       SMTP-отправка (verification code, password reset)
      cache/                      Redis: rate-limit, blacklist, email-коды, password reset
      jwtutil/                    выпуск и подпись JWT (внутренний, отдельно от pkg/jwtutil)
    Dockerfile                    multi-stage build, non-root user

  chat-service/                 чаты, сообщения, presence, typing
    cmd/server/main.go
    internal/
      domain/                     Chat, Message, ChatMember + ошибки
      repository/                 Postgres-репозиторий (чаты, сообщения, event log)
      service/                    бизнес-логика чатов, шифрование-агностично
      transport/grpc/             gRPC-сервер + auth/internal-интерцептор
      cache/                      Redis: presence (online/last-seen), typing, rate-limit
      events/                     публикация в NATS (msg.created, msg.updated, ...)
      authclient/                 gRPC-клиент к auth-service (UserExists)
    Dockerfile

  ws-gateway/                   держит WebSocket-соединения
    cmd/server/main.go
    internal/
      ws/                          handler (upgrade, origin-allowlist), session, registry, fanout
      chatclient/                  gRPC-клиент к chat-service
      events/                     NATS-консьюмер (JetStream + core pub/sub)
    Dockerfile

  notification-worker/          решает, кому и когда слать push
    cmd/worker/main.go
    internal/
      events/                     NATS-консьюмер входящих событий
      notify/                     логика "нужно ли уведомлять" + публикация notify.push
      chatclient/                  gRPC-клиент к chat-service
    Dockerfile

proto/
  auth/v1/auth.proto             контракт auth-service (+ HTTP-аннотации для grpc-gateway)
  chat/v1/chat.proto             контракт chat-service
  gen/                            сгенерированный код — отдельный Go-модуль, НЕ редактировать руками
    auth/v1/, chat/v1/             *.pb.go, *_grpc.pb.go, *.pb.gw.go
    openapi/                       messenger.swagger.json (публикуется на GitHub Pages)
  third_party/google/api/         google/api/annotations.proto (зависимость для HTTP-аннотаций)

pkg/
  jwtutil/                       офлайн-валидация JWT без похода в auth-service —
                                  используется chat-service и ws-gateway

frontend/                       ванильный JS без сборки, раздаётся nginx напрямую
  index.html
  js/
    main.js                       bootstrap, роутинг состояний, все обработчики событий
    api.js                        HTTP-клиент к auth/chat REST-эндпоинтам
    ws.js                          WebSocket-клиент, реконнект с backoff
    crypto.js                      RSA/AES E2E-шифрование через Web Crypto API
    state.js                       единый объект состояния приложения
    avatar.js, theme.js             вспомогательные модули
    screens/                      auth.js, sidebar.js, conversation.js, settings.js, toast.js
  styles/                        app.css, theme.css

nginx/
  nginx.conf                     dev-конфиг, самоподписанный сертификат, порты 8090/8443
  nginx.prod.conf                прод-конфиг, Let's Encrypt, стандартные порты 80/443
  certs/                         самоподписанные dev-сертификаты (не в репозитории)

.github/workflows/
  ci.yml                        сборка + тесты каждого сервиса отдельно (матрица)
  pages.yml                     публикация Swagger UI + coverage на GitHub Pages
```

## Быстрый старт (локально)

Понадобится Docker и Docker Compose.

```bash
git clone https://github.com/VladimirKhmelev/messenger-on-go.git
cd messenger-on-go
make up          # docker-compose up -d --build
```

Открой **https://localhost:8443** (самоподписанный сертификат — браузер
попросит подтвердить исключение безопасности при первом заходе). Локально
письма верификации/сброса пароля перехватывает Mailhog — смотри их на
**http://localhost:8025**.

Остановить: `make down`.

## Другие команды

```bash
make proto         # регенерировать proto/gen из .proto файлов
make unit          # unit-тесты + coverage-репорт по всем сервисам
make integration   # integration-тесты (build tag `integration`)
make lint          # protolint + golangci-lint по всем сервисам
make ci            # полный набор проверок, как в GitHub Actions
```

## Деплой на прод

Продовая конфигурация отличается от локальной только набором переменных
окружения и nginx-конфигом — сам `docker-compose.yml` один и тот же.

1. **VPS** с Docker + Docker Compose, Ubuntu 24.04 LTS.
2. **DNS**: A-запись домена на IP сервера.
3. **TLS**: получить сертификат Let's Encrypt на хосте (`certbot certonly
   --standalone -d yourdomain.com`) — порт 80 должен быть свободен на момент
   получения.
4. **`.env`** в корне репозитория на сервере:
   ```
   JWT_SECRET=<случайная строка, openssl rand -hex 32>
   INTERNAL_SECRET=<случайная строка, openssl rand -hex 32>
   COOKIE_SECURE=true
   ALLOWED_ORIGINS=https://yourdomain.com

   GITHUB_CLIENT_ID=<из GitHub OAuth App>
   GITHUB_CLIENT_SECRET=<из GitHub OAuth App>

   SMTP_ADDR=smtp-relay.brevo.com:587
   SMTP_FROM=noreply@yourdomain.com
   SMTP_DISPLAY_NAME=YourAppName
   SMTP_USERNAME=<из SMTP-провайдера>
   SMTP_PASSWORD=<из SMTP-провайдера>

   NGINX_HTTP_PORT=80
   NGINX_HTTPS_PORT=443
   NGINX_CONF=./nginx/nginx.prod.conf
   LETSENCRYPT_DIR=/etc/letsencrypt
   ```
5. Во фронтенде (`frontend/index.html`) прописать `window.WISP_GITHUB_CLIENT_ID`
   на реальный GitHub OAuth `client_id` (он публичный)
6. `docker compose up -d --build`.

Автопродление Let's Encrypt настраивается certbot'ом автоматически
(systemd timer), никаких дополнительных действий не требуется.

## Тестовая инфраструктура

- Unit-тесты — рядом с кодом в каждом пакете.
- Integration-тесты — в тех же пакетах под build tag `integration`
  (testcontainers, реальный Postgres/Redis).
- CI (GitHub Actions) — сборка и тесты каждого сервиса отдельно,
  результаты и Swagger UI публикуются на GitHub Pages при мёрже в `main`.
