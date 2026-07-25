# VlessPanel

Веб-приложение для управления 3X-UI панелями: клиенты, VLESS-ключи, подписки и тестирование ключей.

## Состав

| Компонент        | Технология       | Порт   |
| ---------------- | ---------------- | ------ |
| Backend          | Go + chi         | 8080   |
| Frontend         | React + Vite     | —      |
| VlessSubTest     | Go daemon        | 7070   |

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
