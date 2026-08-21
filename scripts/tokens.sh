#!/usr/bin/env bash
# Управление API-токенами VlessPanel (для ботов/агентов).
#
# Требует VLESSPANEL_ADMIN_TOKEN в окружении (master-токен из env бэкенда).
# Базовый URL — VLESSPANEL_BASE или http://localhost:9090.
#
# Использование:
#   ./tokens.sh issue <label>      — выпустить токен (raw показывается один раз)
#   ./tokens.sh list               — список выпущенных токенов
#   ./tokens.sh revoke <id>        — отозвать токен
set -euo pipefail

BASE="${VLESSPANEL_BASE:-http://localhost:9090}"
ADMIN="${VLESSPANEL_ADMIN_TOKEN:-}"

if [[ -z "$ADMIN" ]]; then
  echo "error: VLESSPANEL_ADMIN_TOKEN не задан" >&2
  exit 1
fi

cmd="${1:-}"
shift || true

case "$cmd" in
  issue)
    label="${1:?usage: $0 issue <label>}"
    curl -sS -X POST \
      -H "Authorization: Bearer $ADMIN" \
      -H "Content-Type: application/json" \
      -d "{\"label\":\"$label\"}" \
      "$BASE/api/tokens"
    echo
    ;;
  list)
    curl -sS -H "Authorization: Bearer $ADMIN" "$BASE/api/tokens"
    echo
    ;;
  revoke)
    id="${1:?usage: $0 revoke <id>}"
    curl -sS -X DELETE -H "Authorization: Bearer $ADMIN" "$BASE/api/tokens/$id"
    echo
    ;;
  *)
    echo "usage: $0 issue <label> | list | revoke <id>" >&2
    exit 1
    ;;
esac
