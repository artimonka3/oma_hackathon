#!/usr/bin/env bash
# Smoke-тест каркаса: минимальный запуск без реального API ОМА.
# Полный путь: демо-сессии -> проект -> помещение -> дизайн-задание ->
# выбор варианта -> спецификация -> cart-sync (идемпотентность) ->
# preflight -> проверка прораба -> план -> отчёт -> приёмка ->
# котировка/оплата симулятора -> входящее событие в журнале проекта.
set -euo pipefail
cd "$(dirname "$0")/.."

APP_ADDR="127.0.0.1:18080"
MOCK_ADDR="127.0.0.1:18090"
WORKER_ADDR="127.0.0.1:18091"
export OMA_APP_BASE_URL="http://$APP_ADDR"

cleanup() {
  [ -n "${APP_PID:-}" ] && kill "$APP_PID" 2>/dev/null || true
  [ -n "${MOCK_PID:-}" ] && kill "$MOCK_PID" 2>/dev/null || true
  [ -n "${WORKER_PID:-}" ] && kill "$WORKER_PID" 2>/dev/null || true
}
trap cleanup EXIT

step() { printf '\n== %s\n' "$1"; }
fail() { echo "FAIL: $1" >&2; exit 1; }

step "Сборка"
go build -o bin/ ./cmd/... || fail "сборка"
[ -x bin/app ] || fail "bin/app не создан"

step "Запуск процессов"
OMA_API_BASE_URL="http://$MOCK_ADDR" APP_ADDR="$APP_ADDR" bin/app >var/app.log 2>&1 & APP_PID=$!
MOCKOMA_ADDR="$MOCK_ADDR" bin/mockoma >var/mockoma.log 2>&1 & MOCK_PID=$!
WORKER_ADDR="$WORKER_ADDR" bin/worker >var/worker.log 2>&1 & WORKER_PID=$!
mkdir -p var
sleep 1
curl -sf "http://$APP_ADDR/healthz" >/dev/null || { cat var/app.log; fail "app не поднялся"; }
curl -sf "http://$MOCK_ADDR/healthz" >/dev/null || { cat var/mockoma.log; fail "mockoma не поднялся"; }
curl -sf "http://$WORKER_ADDR/healthz" | jq -e '.status=="ok"' >/dev/null || fail "worker не поднялся"
echo "app, mockoma, worker работают"

step "Демо-сессии"
CLIENT=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/demo/session" -d '{"participant":"client"}' | jq -r .token)
FOREMAN=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/demo/session" -d '{"participant":"foreman"}' | jq -r .token)
[ "$CLIENT" != "null" ] && [ "$FOREMAN" != "null" ] || fail "демо-сессии"
CA="Authorization: Bearer $CLIENT"
FA="Authorization: Bearer $FOREMAN"

step "Проект и помещение"
PID=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects" -H "$CA" \
  -d '{"goal":"Обновить гостиную и прихожую","budget_minor":900000,"execution_mode":"foreman"}' | jq -r .id)
ROOM=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/rooms" -H "$CA" \
  -d '{"name":"Гостиная","width_mm":4000,"length_mm":5000,"height_mm":2700,"dimensions_confirmed":true}' | jq -r .id)
[ "$PID" != "null" ] && [ "$ROOM" != "null" ] || fail "проект/помещение"

step "Загрузка фото (медиа)"
echo "демо-план" > var/plan.txt
MEDIA=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/captures" -H "$CA" \
  -F "file=@var/plan.txt" -F "kind=plan" | jq -r .id)
[ "$MEDIA" != "null" ] || fail "медиа"

step "Подсказки из истории покупок (без реального API ОМА)"
LOTS=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/inventory-suggestions" -H "$CA" | jq '.lots | length')
[ "$LOTS" -ge 1 ] || fail "подсказки истории"

step "Дизайн-задание через worker"
JOB=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/design-jobs" -H "$CA" | jq -r .id)
sleep 1
STATE=$(curl -sf "http://$APP_ADDR/api/v1/jobs/$JOB" -H "$CA" | jq -r .state)
[ "$STATE" = "succeeded" ] || fail "задание в состоянии $STATE"
curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/design-selections" -H "$CA" \
  -d '{"variant_id":"variant_balance"}' >/dev/null || fail "выбор варианта"

step "Спецификация и корзина"
SPEC=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/specifications" -H "$CA")
TOTAL=$(echo "$SPEC" | jq -r .total_minor)
[ "$TOTAL" -gt 0 ] || fail "расчёт спецификации"
CART=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/cart-sync" -H "$CA")
CART_ID=$(echo "$CART" | jq -r .cart_id)
CART_V1=$(echo "$CART" | jq -r .cart_version)
CART2=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/cart-sync" -H "$CA")
CART_V2=$(echo "$CART2" | jq -r .cart_version)
[ "$CART_V2" = "$((CART_V1+1))" ] || fail "корзина не пересинхронизировалась"
LINES=$(curl -sf "http://$MOCK_ADDR/oma-api/v1/carts/$CART_ID" | jq '.lines | length')
[ "$LINES" = "1" ] || fail "повтор cart-sync удвоил строки ($LINES)"
echo "cart-sync идемпотентен: cart=$CART_ID lines=$LINES total=$(echo "$CART" | jq -r .total_minor)"

step "Preflight и проверка прораба"
curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/purchase-preflight" -H "$CA" | jq -e '.status=="ready"' >/dev/null || fail "preflight"
REV=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/reviews" -H "$CA" \
  -H "Idempotency-Key: rev-1" -d '{"foreman_id":"foreman_demo_01","scope":"project"}')
REV_ID=$(echo "$REV" | jq -r .review.id)
DUP=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/reviews" -H "$CA" \
  -H "Idempotency-Key: rev-2" -d '{"foreman_id":"foreman_demo_01","scope":"project"}')
[ "$(echo "$DUP" | jq -r .created)" = "false" ] || fail "дублирующая карточка проверки"
echo "повторный запрос вернул ту же карточку ($REV_ID)"
curl -sf -XPOST "http://$APP_ADDR/api/v1/reviews/$REV_ID/decisions" -H "$FA" \
  -d '{"action":"approve_current","comment":"Состав подтверждён"}' | jq -e '.status=="approved"' >/dev/null \
  || fail "решение прораба"
CODE=$(curl -s -XPOST "http://$APP_ADDR/api/v1/reviews/$REV_ID/decisions" -H "$FA" \
  -d '{"action":"approve_current"}' -o /dev/null -w '%{http_code}')
[ "$CODE" = "409" ] || fail "ожидался 409 INVALID_TRANSITION, получен $CODE"

step "План работ, отчёт и приёмка"
PLAN=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/projects/$PID/work-plans" -H "$CA" \
  -d '{"stages":[{"title":"Подготовка стен","room_id":"'"$ROOM"'","weight":60,"required":true},{"title":"Покраска","room_id":"'"$ROOM"'","weight":40,"required":true}]}')
PLAN_ID=$(echo "$PLAN" | jq -r .id)
STAGE=$(echo "$PLAN" | jq -r '.stage_ids[0]')
curl -sf -XPOST "http://$APP_ADDR/api/v1/work-plans/$PLAN_ID/confirmation" -H "$CA" >/dev/null || fail "подтверждение плана"
curl -sf -XPOST "http://$APP_ADDR/api/v1/stages/$STAGE/start" -H "$FA" >/dev/null || fail "старт этапа"
REP=$(curl -sf -XPOST "http://$APP_ADDR/api/v1/stages/$STAGE/reports" -H "$FA" \
  -d '{"items":["Стены зашпаклёваны","Грунт нанесён"],"comment":"Готово к покраске"}')
REP_ID=$(echo "$REP" | jq -r .report.id)
STAGE_V=$(echo "$REP" | jq -r .stage.version)
CODE=$(curl -s -XPOST "http://$APP_ADDR/api/v1/stages/$STAGE/decisions" -H "$CA" \
  -d '{"expected_version":'"$STAGE_V"',"report_id":"'"$REP_ID"'","decision":"accept","acceptance_confirmed":false}' \
  -o /dev/null -w '%{http_code}')
[ "$CODE" = "400" ] || fail "приёмка без явного подтверждения должна давать 400, получено $CODE"
curl -sf -XPOST "http://$APP_ADDR/api/v1/stages/$STAGE/decisions" -H "$CA" \
  -d '{"expected_version":'"$STAGE_V"',"report_id":"'"$REP_ID"'","decision":"accept","acceptance_confirmed":true,"comment":"Замечаний нет, этап принимаю"}' \
  | jq -e '.status=="accepted"' >/dev/null || fail "приёмка этапа"
CODE=$(curl -s -XPOST "http://$APP_ADDR/api/v1/stages/$STAGE/decisions" -H "$FA" \
  -d '{"expected_version":99,"decision":"accept","acceptance_confirmed":true}' -o /dev/null -w '%{http_code}')
[ "$CODE" = "403" ] || fail "прораб не должен принимать этап, получено $CODE"
echo "этап принят, попытка приёмки прорабом отклонена (403)"

step "Оформление и оплата симулятора (события в журнал проекта)"
QUOTE=$(curl -sf -XPOST "http://$MOCK_ADDR/oma-api/v1/carts/$CART_ID/quotes" | jq -r .id)
CHK=$(curl -sf -XPOST "http://$MOCK_ADDR/oma-api/v1/checkout-sessions" -d '{"quote_id":"'"$QUOTE"'"}' | jq -r .id)
ORDER=$(curl -sf -XPOST "http://$MOCK_ADDR/oma-api/v1/checkout-sessions/$CHK/pay")
ORDER_ID=$(echo "$ORDER" | jq -r .id)
[ "$(echo "$ORDER" | jq -r .status)" = "paid" ] || fail "оплата симулятора"
PAID_EVENTS=$(curl -sf "http://$APP_ADDR/api/v1/projects/$PID/events" -H "$CA" | jq '[.events[] | select(.type=="order.paid")] | length')
[ "$PAID_EVENTS" -ge 1 ] || fail "событие оплаты не доставлено в журнал проекта"
echo "заказ $ORDER_ID оплачен, событие order.paid попало в журнал проекта"

step "Недоступность магазина"
OMA_API_BASE_URL="http://127.0.0.1:59999" APP_ADDR="127.0.0.1:18081" bin/app >var/app2.log 2>&1 & APP2_PID=$!
sleep 1
CT=$(curl -sf -XPOST "http://127.0.0.1:18081/api/v1/demo/session" -d '{"participant":"client"}' | jq -r .token)
PID2=$(curl -sf -XPOST "http://127.0.0.1:18081/api/v1/projects" -H "Authorization: Bearer $CT" -d '{"goal":"Тест недоступности"}' | jq -r .id)
curl -sf -XPOST "http://127.0.0.1:18081/api/v1/projects/$PID2/rooms" -H "Authorization: Bearer $CT" \
  -d '{"name":"Спальня","width_mm":3000,"length_mm":4000,"height_mm":2600,"dimensions_confirmed":true}' >/dev/null
CODE=$(curl -s -XPOST "http://127.0.0.1:18081/api/v1/projects/$PID2/design-jobs" \
  -H "Authorization: Bearer $CT" -o /dev/null -w '%{http_code}')
[ "$CODE" = "503" ] || fail "недоступность магазина должна давать 503 INTEGRATION_UNAVAILABLE, получено $CODE"
kill "$APP2_PID" 2>/dev/null || true
echo "при недоступности API ОМА проект сохранён, интеграция честно отвечает 503"

printf '\nSMOKE OK: приложения собираются, HTTP-серверы работают, доменные правила соблюдены.\n'
