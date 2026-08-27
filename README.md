# VlessPanel

Веб-приложение для управления 3X-UI панелями: клиенты, VLESS-ключи, KeySources, подписки и тестирование ключей.

## Состав

| Компонент        | Технология       | Порт   |
| ---------------- | ---------------- | ------ |
| Backend          | Go + chi         | 8080   |
| Frontend         | React + Vite     | —      |
| VlessSubTest     | Go daemon        | 7070   |

## Архитектура

```
                         ┌──────────────────────────────────────┐
                         │  React Frontend (Vite, тёмная тема)  │
                         │  hooks/useVlessPanel + компоненты    │
                         └──────────────┬───────────────────────┘
                                        │ /api  (Authorization: Bearer)
                                        ▼
┌──────────────────────┐   ┌───────────────────────────────────────┐   ┌──────────────────────┐
│  3X-UI Panel         │◄──│  Go Backend (:8080)                   │──►│  VlessSubTest daemon │
│  (v3.3.1–3.6.0)      │   │  auth: bearer-токены (master + API)   │   │  (:7070, sing-box)   │
└──────────────────────┘   │  service-слой: panel / subscription / │   └──────────────────────┘
                           │               keysource / sync / token│
                           │  handlers + chi-роутер, статика (SPA) │
                           └───────┬──────────────────────┬────────┘
                                   │                      │
                     REST к 3X-UI  ▼                      ▼ rsync-скрипт (POST /sync)
                           Файлы на диске:              ┌──────────────────────┐
                           panels.json                  │  Aggregator          │
                           key-sources.json             │  configs-*.txt       │
                           subscriptions.json           │  (VLESSPANEL_SYNC_   │
                           tokens.json (sha256)         │   SCRIPT)            │
                           configs-*.txt                └──────────────────────┘
```

- **Единый бэкенд** — Go-бинарник раздаёт и `/api/*`, и собранную статику фронта (SPA-fallback на `index.html`).
- **Аутентификация** — включается переменной `VLESSPANEL_ADMIN_TOKEN` (master-токен). При включённой auth все `/api/*` требуют `Authorization: Bearer <токен>`; `GET /api/auth-status` и `GET /api/config` — без токена. Master-токен имеет полный доступ; выпущенные через `POST /api/tokens` токены — доступ к panel/subscription/keysource API, но не к `/api/tokens`. Токены хранятся в `data/tokens.json` только как sha256-хэши.
- **Токены панелей** хранятся только на сервере в `panels.json` и фронту не отдаются: backend — единственный клиент 3X-UI.
- **KeySource-модель** — источник ключа: `panel` (клиент 3X-UI: panelId + clientEmail + inboundId) или `manual` (готовый `vless://`). Подписки создаются из `keySourceIds` с дедупликацией по типу; актуальные ключи для panel-KeySource фетчатся с 3X-UI при генерации.
- **Подписка = файл** `configs-{name}.txt` в папке агрегатора: одна подписка — один человек, каждая строка — один `vless://` ключ. Публичная ссылка на подписку: `{VLESSPANEL_PUBLIC_URL}/sub/{name}` (отдаётся фронту через `GET /api/config`).
- **Синхронизация с агрегатором** — `POST /api/sync` запускает rsync-скрипт (`VLESSPANEL_SYNC_SCRIPT`); флаг «нужен синк» поднимается при изменении файлов подписок (см. `syncstate.go`).
- **Тестирование ключей** делегируется внешнему демону VlessSubTest (`POST /test-single`), который использует sing-box.
- **Совместимость с 3X-UI**: для списка клиентов используется эндпоинт `/panel/api/clients/list` (v3.4.2+); спецификации OpenAPI для версий 3.3.1 / 3.4.2 / 3.5.0 / 3.6.0 лежат в `spec/3xui/`.

## Структура проекта

```
VlessPanelWebApp/
├── backend/                      # Go 1.23 (пакет main + подпакеты, роутер chi)
│   ├── main.go                   # точка входа: конфиг, роутер, маршруты /api/*, статика
│   ├── config.go                 # Config + LoadConfig() из переменных окружения VLESSPANEL_*
│   ├── auth.go                   # bearer-аутентификация: master-токен + выпуск API-токенов
│   ├── handlers.go               # Handlers: HTTP-обработчики всех endpoint'ов
│   ├── middleware.go             # логирование запросов + CORS
│   ├── interfaces.go             # Repository / PanelClient / VlessSubTestClient / AggregatorSyncer
│   ├── service_panel.go          # service-слой: панели и клиенты 3X-UI
│   ├── service_subscription.go   # service-слой: подписки и генерация файлов
│   ├── service_keysource.go      # service-слой: KeySources (panel/manual)
│   ├── service_sync.go           # service-слой: синк с агрегатором
│   ├── service_token.go          # service-слой: выпуск/отзыв API-токенов
│   ├── storage.go                # Storage: файловое хранилище (panels.json и др.), RWMutex
│   ├── panelapi.go               # HTTP-клиент к 3X-UI, сборка vless-ссылок, CRUD клиентов
│   ├── aggregator.go             # запуск rsync-скрипта синхронизации
│   ├── syncstate.go              # atomic-флаг «нужен синк с агрегатором»
│   ├── vlesssubtest.go           # клиент демона VlessSubTest (/test, /test-single)
│   ├── errors.go                 # sentinel-ошибки для errors.Is
│   ├── messages.go               # user-facing сообщения (единый русский)
│   ├── helpers.go                # общие утилиты
│   ├── *_test.go                 # unit-тесты сервисов/хранилища/панелей/авторизации
│   ├── model/model.go            # доменные типы (Panel, Client, KeySource, Subscription, APIToken)
│   ├── dto/dto.go                # запросы/ответы API и контракты 3X-UI/демона
│   ├── xui/xui.go                # типы API 3X-UI
│   └── go.mod / go.sum
├── frontend/                     # React 18 + Vite 6 (без UI-библиотек и стейт-менеджмента)
│   ├── index.html                # входной HTML, #root
│   ├── vite.config.js            # dev-прокси /api → :8080
│   └── src/
│       ├── main.jsx              # монтирование React
│       ├── App.jsx               # маршрутизация вкладок, композиция состояния
│       ├── api.js                # fetch-обёртка над /api/* + publicUrl из /api/config
│       ├── hooks/useVlessPanel.js # хук: всё состояние и действия приложения
│       ├── components/           # по файлу на компонент: Header, ClientCard, AuthGate,
│       │                         # Toast, Modal, AddPanelModal, AddClientModal, KSDetailsModal,
│       │                         # SubscriptionDetail, ReportModal, …, format.js
│       └── App.css               # тёмная тема
├── data/                         # runtime-данные (gitignored)
│   ├── panels.json               # список 3X-UI панелей (url, token, webBasePath)
│   ├── key-sources.json          # KeySources
│   ├── subscriptions.json        # метаданные подписок
│   ├── tokens.json               # выпущенные API-токены (только sha256-хэши)
│   ├── aggregator/               # файлы подписок configs-{name}.txt
│   └── vlesssubtest/             # results.db (bbolt), config.json (крон)
├── spec/                         # спецификация и контракты
│   ├── keysource-redesign.md     # ТЗ редизайна KeySources
│   ├── keysource-mockup.html     # макет UI KeySources
│   └── 3xui/{3.3.1,3.4.2,3.5.0,3.6.0}/openapi.json  # контракты API 3X-UI
├── scripts/tokens.sh             # выпуск/список/отзыв API-токенов (нужен master-токен)
├── Dockerfile                    # multi-stage: node → golang → alpine runtime
├── docker-compose.yml            # vlesspanel (:9090→8080) + vlesssubtest (:7070), сеть vlesspanel-net
└── AGENTS.md                     # процедура пересборки, деплоя и smoke-тестов
```

## Требования

- Docker + docker compose v2

## Быстрый запуск

```bash
cd VlessPanelWebApp
docker compose up -d --build
```

После запуска:
- Веб-интерфейс: **http://localhost:9090**
- VlessSubTest daemon: **http://localhost:7070**

Для остановки:

```bash
docker compose down
```

## Ручной запуск (отладка)

### Backend

```bash
cd backend

# Опционально: указать URL демона
export VLESSPANEL_VLESSSUBTEST_DAEMON_URL=http://localhost:7070

go run .
```

Backend стартует на `http://localhost:8080`.

### Frontend (dev-режим)

```bash
cd frontend
npx vite --host
```

Dev-сервер на `http://localhost:5173` с проксированием API-запросов к backend.

### VlessSubTest daemon

Демон живёт в отдельном репозитории `VlessSubTest`. Запуск готового образа:

```bash
docker run -d --name vlesssubtest \
  -p 7070:7070 \
  --network vlesspanel-net \
  vlesssubtest:latest --port=7070
```

Либо собрать из исходников:

```bash
cd VlessSubTest
go build -o vlesssubtest .
./vlesssubtest --port=7070
```

## Переменные окружения

| Переменная                            | По умолчанию               | Описание                                        |
| ------------------------------------- | -------------------------- | ----------------------------------------------- |
| `VLESSPANEL_PORT`                     | `8080`                     | Порт backend                                    |
| `VLESSPANEL_AGGREGATOR_DIR`           | `/opt/vless-aggregator`    | Папка с файлами подписок                        |
| `VLESSPANEL_PANELS_FILE`              | `panels.json`              | Путь к файлу панелей                            |
| `VLESSPANEL_DATA_DIR`                 | каталог `panels.json`      | Каталог runtime-данных (key-sources и др.)      |
| `VLESSPANEL_STATIC_DIR`               | `../frontend/dist`         | Папка со статикой фронта                        |
| `VLESSPANEL_VLESSSUBTEST_DAEMON_URL`  | `http://vlesssubtest:7070` | URL демона VlessSubTest                         |
| `VLESSPANEL_PUBLIC_URL`               | `https://example.com`      | Публичный базовый URL агрегатора (ссылки `…/sub/{name}`) |
| `VLESSPANEL_SYNC_SCRIPT`              | `/opt/aggregator-configs/sync-configs.sh` | rsync-скрипт синхронизации агрегатора |
| `VLESSPANEL_ADMIN_TOKEN`              | — (пусто)                  | Master-токен; пусто = auth выключена            |

Таймауты HTTP-сервера (значения — Go duration, например `30s`, `2m`):

| Переменная                        | По умолчанию | Описание                                   |
| --------------------------------- | ------------ | ------------------------------------------ |
| `VLESSPANEL_READ_HEADER_TIMEOUT`  | `10s`        | Таймаут чтения заголовков запроса          |
| `VLESSPANEL_READ_TIMEOUT`         | `30s`        | Таймаут чтения запроса                     |
| `VLESSPANEL_WRITE_TIMEOUT`        | `2m`         | Таймаут записи ответа                      |
| `VLESSPANEL_IDLE_TIMEOUT`         | `2m`         | Таймаут простоя keep-alive соединения      |
| `VLESSPANEL_SHUTDOWN_TIMEOUT`     | `8s`         | Время на graceful shutdown                 |

## Аутентификация

- Включается переменной `VLESSPANEL_ADMIN_TOKEN` (master-токен). Пустая переменная → auth выключена.
- При включённой auth на `/api/*` нужен заголовок `Authorization: Bearer <токен>`; без него — `401`.
- `GET /api/auth-status` (включена ли auth) и `GET /api/config` (публичная конфигурация фронта) — без токена.
- Master-токен — полный доступ. Выпущенные токены — доступ к panel/subscription/keysource API, но **не** к `/api/tokens`.
- Выпуск/отзыв токенов: `scripts/tokens.sh issue <label> | list | revoke <id>` (нужны `VLESSPANEL_ADMIN_TOKEN` и `VLESSPANEL_BASE`, по умолчанию `http://localhost:9090`).

```bash
curl -s http://localhost:9090/api/panels -H "Authorization: Bearer $VLESSPANEL_ADMIN_TOKEN"
```

## API Endpoints

### Панели и клиенты

| Метод   | Путь                                        | Описание                              |
| ------- | ------------------------------------------- | ------------------------------------- |
| GET     | `/api/panels`                               | Список панелей                        |
| POST    | `/api/panels`                               | Добавить панель                       |
| DELETE  | `/api/panels/:id`                           | Удалить панель                        |
| GET     | `/api/panels/:id/clients`                   | Клиенты панели                        |
| POST    | `/api/panels/:id/clients`                   | Создать клиента                       |
| GET     | `/api/panels/:id/clients/:email/keys`       | VLESS-ключи клиента по всем инбаундам |
| POST    | `/api/panels/:id/clients/:email/attach`     | Привязать инбаунд                     |
| POST    | `/api/panels/:id/clients/:email/detach`     | Отвязать инбаунд                      |
| POST    | `/api/panels/:id/clients/:email/update`     | Обновить expiryDate клиента           |
| POST    | `/api/panels/:id/inbounds`                  | Инбаунды панели (именно POST)         |

### Key Sources

| Метод   | Путь                            | Описание                                        |
| ------- | ------------------------------- | ----------------------------------------------- |
| GET     | `/api/key-sources`              | Список KeySources                               |
| POST    | `/api/key-sources`              | Создать (`type: "panel"` / `"manual"`, дедуп)   |
| GET     | `/api/key-sources/:id`          | Один KeySource                                  |
| DELETE  | `/api/key-sources/:id`          | Удалить (+ каскадная чистка из подписок)        |
| GET     | `/api/key-sources/:id/key`      | Свежий ключ (для panel — фетч с 3X-UI)          |
| GET     | `/api/key-sources/:id/test`     | Тест ключа через VlessSubTest                   |
| GET     | `/api/key-sources/:id/traffic`  | up/down/expiry (только panel)                   |

### Подписки

| Метод   | Путь                                  | Описание                            |
| ------- | ------------------------------------- | ----------------------------------- |
| GET     | `/api/subscriptions`                  | Список подписок                     |
| POST    | `/api/subscriptions`                  | Создать из `keySourceIds` или `keys`|
| POST    | `/api/subscriptions/regenerate-all`   | Перегенерить все подписки           |
| GET     | `/api/subscriptions/:id`              | Детали подписки                     |
| PUT     | `/api/subscriptions/:id`              | Обновить подписку                   |
| DELETE  | `/api/subscriptions/:id`              | Удалить подписку                    |
| GET     | `/api/subscriptions/:id/raw`          | Сырое содержимое файла              |
| POST    | `/api/subscriptions/:id/test`         | Тест всех ключей подписки           |

### Служебные и токены

| Метод   | Путь                         | Описание                                    |
| ------- | ---------------------------- | ------------------------------------------- |
| POST    | `/api/sync`                  | rsync-синк файлов подписок на агрегатор     |
| GET     | `/api/vlesssubtest-status`   | Статус демона тестирования                  |
| GET     | `/api/auth-status`           | Включена ли auth (без токена)               |
| GET     | `/api/config`                | Публичная конфигурация фронта (без токена)  |
| GET     | `/api/tokens`                | Список выпущенных API-токенов (master)      |
| POST    | `/api/tokens`                | Выпустить API-токен (master)                |
| DELETE  | `/api/tokens/:id`            | Отозвать API-токен (master)                 |

## Тестирование ключей

Тестирование выполняется через демон VlessSubTest (`POST /test-single`).

Для полноценной работы тестов демону нужен **sing-box** внутри контейнера.
Без sing-box тесты будут возвращать `SING_BOX_START_FAILED`.

Ответ API:

```json
{
  "total": 2,
  "ok": 1,
  "results": [
    {
      "key_idx": 0,
      "ip": "1.1.1.1",
      "remark": "MyServer",
      "status": "OK",
      "youtube": "OK",
      "instagram": "OK"
    }
  ]
}
```

## Сборка

```bash
# Backend
cd backend && go build -o vlesspanel .

# Frontend
cd frontend && npx vite build
```
