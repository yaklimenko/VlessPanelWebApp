# VlessPanel

Веб-приложение для управления 3X-UI панелями: клиенты, VLESS-ключи, подписки и тестирование ключей.

## Состав

| Компонент        | Технология       | Порт   |
| ---------------- | ---------------- | ------ |
| Backend          | Go + chi         | 8080   |
| Frontend         | React + Vite     | —      |
| VlessSubTest     | Go daemon        | 7070   |

## Архитектура

```
┌─────────────────────┐       ┌───────────────────────────────────┐        ┌────────────────┐
│  React Frontend     │──────→│  Go Backend (:8080)               │──────→│  3X-UI Panel   │
│  (Vite, тёмная тема)│  /api │  - панели, клиенты, ключи        │  REST │  (v3.4.2/3.5.0)│
└─────────────────────┘       │  - подписки (CRUD файлов)         │       └────────────────┘
                              │  - статика фронта (SPA fallback)  │
                              │          │                        │
                              │          ▼                        │        ┌────────────────┐
                              │  Файлы на диске:                  │       │ VlessSubTest   │
                              │  panels.json, configs-*.txt       │──────→│ daemon (:7070)  │
                              └───────────────────────────────────┘ /test-single└────────────────┘
```

- **Единый бэкенд** — Go-бинарник раздаёт и `/api/*`, и собранную статику фронта (SPA-fallback на `index.html`).
- **Токены панелей** хранятся только на сервере в `panels.json` и фронту не отдаются: backend — единственный клиент 3X-UI.
- **Подписка = файл** `configs-{name}.txt` в папке агрегатора: одна подписка — один человек, каждая строка — один `vless://` ключ.
- **Тестирование ключей** делегируется внешнему демону VlessSubTest (`POST /test-single`), который использует sing-box.
- **Совместимость с 3X-UI**: для списка клиентов сначала пробуется эндпоинт `/panel/api/clients/list` (v3.4.2+), при неудаче — фолбэк на разбор inbounds.

## Структура проекта

```
VlessPanelWebApp/
├── backend/                      # Go 1.23 (единый пакет main, роутер chi)
│   ├── main.go                   # точка входа: конфиг, роутер, маршруты /api/*, статика
│   ├── config.go                 # Config + LoadConfig() из переменных окружения VLESSPANEL_*
│   ├── types.go                  # доменные типы (Panel, Client, VLESSKey, Subscription) и DTO 3X-UI
│   ├── storage.go                # Storage: файловое хранилище (panels.json, configs-*.txt), RWMutex
│   ├── panelapi.go               # PanelAPI: HTTP-клиент к 3X-UI, сборка vless-ссылок, CRUD клиентов
│   ├── handlers.go               # Handlers: HTTP-обработчики всех endpoint'ов
│   ├── middleware.go             # логирование запросов + CORS
│   └── go.mod / go.sum
├── frontend/                     # React 18 + Vite (без UI-библиотек и стейт-менеджмента)
│   ├── index.html                # входной HTML, #root
│   ├── vite.config.js            # dev-прокси /api → :8080
│   └── src/
│       ├── main.jsx              # монтирование React
│       ├── App.jsx               # всё состояние и логика приложения
│       ├── api.js                # fetch-обёртка над /api/*
│       ├── components/index.jsx  # Toast, Modal, Header, ClientCard, SubscriptionCard, модалки
│       └── App.css               # тёмная тема
├── data/                         # runtime-данные (gitignored)
│   ├── panels.json               # список 3X-UI панелей (url, token, webBasePath)
│   ├── aggregator/               # файлы подписок configs-{name}.txt
│   └── vlesssubtest/             # results.db (bbolt), config.json (крон)
├── spec/                         # спецификация и контракты
│   ├── Task.md                   # ТЗ + утверждённая архитектура
│   ├── vlesspanel-mockup.html    # макет UI
│   └── 3xui/{3.4.2,3.5.0}/openapi.json  # контракты API 3X-UI
├── Dockerfile                    # multi-stage: node → golang → alpine runtime
├── docker-compose.yml            # vlesspanel (:9090→8080) + vlesssubtest (:7070), сеть vlesspanel-net
├── AGENTS.md                     # процедура пересборки и деплоя
└── badgoodvless.txt              # заметки о различиях ключей (панель vs 3X-UI)
```

## Требования

- Docker + docker-compose v2

## Быстрый запуск

```bash
cd VlessPanelWebApp
docker-compose up -d --build
```

После запуска:
- Веб-интерфейс: **http://localhost:9090**
- VlessSubTest daemon: **http://localhost:7070**

Для остановки:

```bash
docker-compose down
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

| Переменная                            | По умолчанию               | Описание                          |
| ------------------------------------- | -------------------------- | --------------------------------- |
| `VLESSPANEL_PORT`                     | `8080`                     | Порт backend                      |
| `VLESSPANEL_AGGREGATOR_DIR`           | `/opt/vless-aggregator`    | Папка с файлами подписок          |
| `VLESSPANEL_PANELS_FILE`              | `panels.json`              | Путь к файлу панелей              |
| `VLESSPANEL_STATIC_DIR`               | `../frontend/dist`         | Папка со статикой фронта          |
| `VLESSPANEL_VLESSSUBTEST_DAEMON_URL`  | `http://vlesssubtest:7070` | URL демона VlessSubTest           |

## API Endpoints

### Панели

| Метод   | Путь                                  | Описание                    |
| ------- | ------------------------------------- | --------------------------- |
| GET     | `/api/panels`                         | Список панелей              |
| POST    | `/api/panels`                         | Добавить панель             |
| DELETE  | `/api/panels/:id`                     | Удалить панель              |
| GET     | `/api/panels/:id/clients`             | Клиенты панели              |
| POST    | `/api/panels/:id/clients`             | Создать клиента             |
| GET     | `/api/panels/:id/clients/:email/keys` | VLESS-ключи клиента         |
| POST    | `/api/panels/:id/inbounds`            | Инбаунды панели             |

### Подписки

| Метод   | Путь                              | Описание                    |
| ------- | --------------------------------- | --------------------------- |
| GET     | `/api/subscriptions`              | Список подписок             |
| POST    | `/api/subscriptions`              | Создать подписку            |
| GET     | `/api/subscriptions/:id`          | Детали подписки             |
| PUT     | `/api/subscriptions/:id`          | Обновить подписку           |
| DELETE  | `/api/subscriptions/:id`          | Удалить подписку            |
| GET     | `/api/subscriptions/:id/raw`      | Сырое содержимое            |
| POST    | `/api/subscriptions/:id/test`     | Тест всех ключей подписки   |

### Утилиты

| Метод | Путь                         | Описание                    |
| ----- | ---------------------------- | --------------------------- |
| GET   | `/api/vlesssubtest-status`   | Статус демона тестирования  |

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
