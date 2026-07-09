# VlessPanel

Веб-приложение для управления 3X-UI панелями и подписками VLESS.

## Стек

- **Backend:** Go + chi router
- **Frontend:** React + Vite
- **Данные:** JSON-файлы (panels.json) + txt-файлы подписок

## Структура

```
VlessPanelApp/
├── backend/          # Go backend
│   ├── main.go       # точка входа, роутер, раздача статики
│   ├── handlers.go   # HTTP-обработчики
│   ├── types.go      # типы данных
│   ├── config.go     # конфигурация (env / defaults)
│   ├── storage.go    # работа с файлами (panels, subscriptions)
│   ├── panelapi.go   # 3X-UI API клиент
│   └── middleware.go # CORS, логирование
├── frontend/         # React + Vite
│   ├── src/
│   │   ├── App.jsx
│   │   ├── App.css
│   │   ├── api.js    # API-клиент
│   │   └── components/
│   │       ├── index.jsx  # Header, ClientCard, SubscriptionCard, Modal, Toast
│   │       └── ...
│   └── package.json
├── spec/             # Spec и макет
│   ├── Task.md
│   └── vlesspanel-mockup.html
├── build/            # Сборка (gitignored)
│   └── vlesspanel
├── panels.json       # Панели 3X-UI
├── .gitignore
└── README.md
```

## Быстрый старт

### Dev-сервер

```bash
# Терминал 1: Бэкенд
cd backend && /home/klem/.local/go/bin/go run .

# Терминал 2: Фронт
cd frontend && npx vite --host
```

### Продакшн сборка

```bash
# Фронт
cd frontend && npx vite build

# Бэкенд
cd backend && /home/klem/.local/go/bin/go build -o ../build/vlesspanel .

# Запуск
cd /home/klem/VlessPanelApp && VLESSPANEL_STATIC_DIR=frontend/dist ./build/vlesspanel
```

## API Endpoints

### Панели
- `GET /api/panels` — список
- `POST /api/panels` — добавить `{name, url, token}`
- `DELETE /api/panels/:id` — удалить

### Клиенты (через 3X-UI)
- `GET /api/panels/:id/clients` — список
- `POST /api/panels/:id/clients` — создать `{email, inboundId}`
- `GET /api/panels/:id/clients/:email/keys` — VLESS-ключи
- `POST /api/panels/:id/inbounds` — список инбаундов

### Подписки
- `GET /api/subscriptions` — список
- `POST /api/subscriptions` — создать `{name, keys[]}`
- `PUT /api/subscriptions/:id` — обновить
- `DELETE /api/subscriptions/:id` — удалить
- `GET /api/subscriptions/:id/raw` — содержимое txt
- `POST /api/subscriptions/:id/test` — VlessSubTest

## Конфигурация (env)

| Переменная | По умолчанию | Описание |
|---|---|---|
| `VLESSPANEL_PORT` | `8080` | Порт сервера |
| `VLESSPANEL_AGGREGATOR_DIR` | `/home/klem/VlessAggregator` | Папка с config-*.txt |
| `VLESSPANEL_PANELS_FILE` | `panels.json` | Файл панелей |
| `VLESSPANEL_STATIC_DIR` | `../frontend/dist` | Статика фронта |
| `VLESSPANEL_VLESSSUBTEST_PATH` | `/home/klem/VlessSubTest/vlesssubtest` | VlessSubTest бинарник |

## Тестовая панель

- **Название:** fastVPS Estonia
- **URL:** `https://203.0.113.8:34764/webBasePathPlaceholder1`
- **Token:** в vault (`3xui-fastvps-Estonia-api-token`)

