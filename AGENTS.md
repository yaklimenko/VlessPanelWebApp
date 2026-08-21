## Deploy (поднять сборку)

При запросе «поднять сборку», «пересобрать», «запустить», «задеплоить»:
1. `docker stop vlesspanel && docker rm vlesspanel`
2. `docker compose -f /home/klem/VlessPanelWebApp/docker-compose.yml build vlesspanel`
3. `docker run -d --name vlesspanel --network vlesspanel-net -p 9090:8080 -v /home/klem/VlessPanelWebApp/data:/data -v /opt/aggregator-configs/configs:/opt/aggregator-configs/configs -e VLESSPANEL_PORT=8080 -e VLESSPANEL_AGGREGATOR_DIR=/opt/aggregator-configs/configs -e VLESSPANEL_PANELS_FILE=/data/panels.json -e VLESSPANEL_STATIC_DIR=/app/static -e VLESSPANEL_VLESSSUBTEST_DAEMON_URL=http://vlesssubtest:7070 --restart unless-stopped vlesspanelwebapp-vlesspanel:latest`

Примечание: `docker-compose` недоступен, используется `docker compose` (v2). Тег образа, собираемого compose, — `vlesspanelwebapp-vlesspanel:latest` (через дефис), как и в docker run; при расхождении берите фактический тег из вывода build.

## Аутентификация и API-токены

- Auth включается env `VLESSPANEL_ADMIN_TOKEN` (master-токен). Пустой → auth выключен.
- При включённом auth на `/api` нужен `Authorization: Bearer <токен>`; без него — `401`.
- Master-токен — полный доступ; выпущенные токены — доступ к panel/subscription/keysource API, но **не** к `/api/tokens` (управление — только master).
- `GET /api/auth-status` — без auth, сообщает `{"enabled":true|false}` (фронт решает, показывать ли логин).
- Выпуск/отзыв токенов: `scripts/tokens.sh issue <label> | list | revoke <id>` (нужны `VLESSPANEL_ADMIN_TOKEN` и `VLESSPANEL_BASE`, по умолчанию `http://localhost:9090`).
- Токены хранятся в `data/tokens.json` **только как sha256-хэш**; raw показывается один раз при выпуске.
- Для включения auth в deploy добавьте `-e VLESSPANEL_ADMIN_TOKEN=<секрет>` в команду `docker run`.
- Публичный URL агрегатора (для ссылок `…/sub/<имя>`): env `VLESSPANEL_PUBLIC_URL` (дефолт `https://example.com`), отдаётся фронту через `GET /api/config`.

## Окружение и точки доступа

- Приложение (VlessPanel): `http://localhost:9090` (проксируется в контейнер `vlesspanel`, порт 8080). Контейнер должен быть Up.
- VlessSubTest daemon: `http://localhost:7070` (контейнер `vlesssubtest`), изнутри app-сети — `http://vlesssubtest:7070`.
- Реальные 3X-UI панели хранятся в `data/panels.json` (токены там же). Список: `curl -s http://localhost:9090/api/panels`. Никогда не показывайте/коммитьте токены в логах/issue.
- OpenAPI 3X-UI: `spec/3xui/<version>/openapi.json` (версии 3.3.1, 3.4.2, 3.5.0, 3.6.0). Если нужна свежая версия — попросить пользователя.
- Контейнер-логи: `docker logs vlesspanel --tail 50`. Build-логи и ошибка пересборки — туда же.

## Тестирование изменений (обязательно перед коммитом)

После любого изменения бэкенда — пересобрать контейнер (см. «Deploy») и проверить **затронутый** функционал энд-ту-энд через curl. Тестовые сущности именовать с префиксом `refac-test-*`, после проверки — удалять.

При включённом auth добавляйте к curl `-H "Authorization: Bearer $VLESSPANEL_ADMIN_TOKEN"` (или используйте выпущенный токен).

### Маршруты приложения (http://localhost:9090/api)

Внимание на нестандартные методы:
- `GET  /panels` — список панелей.
- `POST /panels` — добавить панель (body: `{name,url,token,webBasePath?}`).
- `DELETE /panels/{id}` — удалить панель.
- `GET  /panels/{id}/clients` — список клиентов (реальный запрос к 3X-UI).
- `POST /panels/{id}/clients` — создать клиента (body: `{email,inboundId,expiryDate?}` где `expiryDate` = `YYYY-MM-DD`).
- `GET  /panels/{id}/clients/{email}/keys` — VLESS-ключи клиента по всем привязанным инбаундам.
- `POST /panels/{id}/clients/{email}/attach` — привязать инбаунд (body: `{inboundId}`).
- `POST /panels/{id}/clients/{email}/detach` — отвязать инбаунд (body: `{inboundId}`).
- `POST /panels/{id}/clients/{email}/update` — обновить `expiryDate` (body: `{expiryDate}`).
- **`POST /panels/{id}/inbounds`** — список инбаундов (именно **POST**, не GET — известный баг REST, не править без согласования).
- `GET  /key-sources` — список KeySources с derived-статусами.
- `POST /key-sources` — создать (body: `{type:"panel"|"manual", panelId?, clientEmail?, inboundId?, vlessLink?, label?}`). Дедупликация по типу.
- `GET  /key-sources/{id}` — один KeySource.
- `DELETE /key-sources/{id}` — удалить с каскадной чисткой из подписок.
- `GET  /key-sources/{id}/key` — свежий ключ (для panel — фетч с 3X-UI + обновление кэша).
- `GET  /key-sources/{id}/test` — прогон через vlesssubtest, сохраняет `lastTest`.
- `GET  /key-sources/{id}/traffic` — up/down/expiry (только panel).
- `GET  /subscriptions` — список (мета + файлы + sync-статус).
- `POST /subscriptions` — создать из `keySourceIds` или legacy `keys` (body: `{name, keySourceIds?, keys?}`).
- `POST /subscriptions/regenerate-all` — перегенерить все подписки с panel-KeySource.
- `GET  /subscriptions/{id}` — одна подписка.
- `PUT  /subscriptions/{id}` — апдейт (режимы: `regenerate`, `addKeySourceIds`, `removeKeyId`, legacy `keys`, `name` rename).
- `DELETE /subscriptions/{id}` — удалить (файл + мета).
- `GET  /subscriptions/{id}/raw` — сырой файл `configs-{name}.txt`.
- `POST /subscriptions/{id}/test` — тест всех ключей через vlesssubtest.
- `POST /sync` — rsync-скрипт на агрегатор.
- `GET  /vlesssubtest-status` — health демона.
- `GET  /auth-status` — включена ли auth (без токена).
- `GET  /config` — публичная конфигурация фронта (без токена): `{"publicUrl": "..."}`.
- `GET  /tokens` — список выпущенных токенов (только master).
- `POST /tokens` — выпустить токен (только master, body: `{label}`) → `{token (raw), tokenMeta}`.
- `DELETE /tokens/{id}` — отозвать токен (только master).

### Прямой доступ к 3X-UI (для очистки тестовых клиентов)

База: `https://<host>:<port>/<webBasePath>` из `data/panels.json`. Заголовок: `Authorization: Bearer <token>`. `-k` (skip TLS verify).
- `POST /panel/api/clients/del/{email}` — удалить клиента (per OpenAPI 3.6.0).
- `GET  /panel/api/clients/get/{email}` — проверить существование.
- `GET  /panel/api/inbounds/list` — список инбаундов.
- `GET  /panel/api/clients/list` — список клиентов (3.4.2+).
- `GET  /panel/api/clients/links/{email}` — ссылки клиента.

### Smoke-тест (минимум) после пересборки

```bash
# 1. Health-check
curl -s http://localhost:9090/api/vlesssubtest-status
curl -s http://localhost:9090/api/panels | head -c 200

# 2. Полный цикл: test-клиент → ключи → KeySource → подписка → тест → cleanup
# (замените PANEL_ID и INBOUND_ID на реальную панель из /api/panels)
PANEL=adminvps-poland; INB=5; EMAIL=refac-test-$$

# Создаём клиента
curl -s -X POST -H "Content-Type: application/json" \
  -d "{\"email\":\"$EMAIL\",\"inboundId\":$INB,\"expiryDate\":\"2099-01-01\"}" \
  http://localhost:9090/api/panels/$PANEL/clients

# Ключи
curl -s http://localhost:9090/api/panels/$PANEL/clients/$EMAIL/keys | head -c 200

# Cleanup тестового клиента на 3X-UI напрямую (token из panels.json):
TOK=$(python3 -c "import json;p=[x for x in json.load(open('data/panels.json')) if x['id']=='$PANEL'][0];print(p['token'])")
URL=$(python3 -c "import json;p=[x for x in json.load(open('data/panels.json')) if x['id']=='$PANEL'][0];print(p['url']+p['webBasePath'])")
curl -sk -X POST -H "Authorization: Bearer $TOK" "$URL/panel/api/clients/del/$EMAIL"
```

### Чек-лист перед коммитом в `panel-refactoring`

1. `go build ./...` в `backend/` — без ошибок.
2. `gofmt -l backend/*.go` — пусто (или `gofmt -w`).
3. Пересобран контейнер, smoke-тест из блока выше прошёл.
4. Затронутые эндпоинты проверены curl-ом end-to-end (см. «Маршруты»).
5. Тестовые сущности (`refac-test-*`) удалены и с приложения, и с 3X-UI панелей.
6. `git diff` просмотрен, нет отладочных `log.Printf`, закомментированного кода, случайно изменённых токенов.
7. Никаких секретов (токенов панелей) в коммите/логах/diff.

## Ветка рефакторинга

Работа по архитектурному рефакторингу идёт в ветке `panel-refactoring` (от `main`). Коммиты атомарные, на одну тему. Большие шаги (выделение service-слоя, интерфейсы) — отдельными коммитами. Перед PR — полный smoke + проверка, что не сломан существующий UI.

## Известные архитектурные слабости (контекст для рефакторинга)

Осталось (актуально на текущий момент):
- Сервисы/storage/handlers всё ещё в `package main` (типы вынесены в `model`/`dto`/`xui`, но сами слои не в подпакетах). При желании можно разнести `service`/`storage`/`panelapi`/`handlers` по подпакетам.

Уже исправлено в этой ветке: TOCTOU-гонки, `strings.Contains`-диспетчеризация ошибок, цикл Storage↔Handlers (SetOnChange), path traversal, таймауты/graceful shutdown HTTP-сервера, CORS `*`, auth (bearer-token + выпуск токенов), параллельный regenerate-all, service/use-case слой, интерфейсы (Repository/PanelClient/VlessSubTestClient/AggregatorSyncer), разнесение типов по пакетам, per-panel `insecureSkipVerify`, i18n (единый русский), хардкод прод-URL (вынесен в `VLESSPANEL_PUBLIC_URL`), frontend god files (компоненты по файлам + `useVlessPanel`). Добавлены unit-тесты сервисов/хранилища/панелей/авторизации.