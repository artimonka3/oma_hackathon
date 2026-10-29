# Магазин ОМА и ОМА.Подбор

`docker compose up -d --build` запускает postgres → shop → app. Главный адрес:
http://127.0.0.1:8088, магазин через тот же origin: http://127.0.0.1:8088/shop/.
Отдельный HTTP API/витрина доступны на 127.0.0.1:8090 (`SHOP_PORT`).

App использует `oma.Client` (поиск, карточка, история, синхронизация) и `oma.Commerce`
(корзина, котировка, оформление, оплата, заказ, демо-доставка). HTTPClient реализует оба контракта.
Обратная проверка платежа вызывается из shop в app. Разрешение выдаётся только во время
подтверждённой оплаты владельцем, с проверкой quote/cart/project/amount/currency/expiry и хеша
текущего комплекта. Вызовы внутренних маршрутов не держат общий API mutex, чтобы callback не
вызвал взаимную блокировку. События магазина обрабатываются идемпотентно.

Основные маршруты магазина:

| Метод | Путь |
|---|---|
| GET | `/oma-api/v1/products?q=&category=` |
| GET | `/oma-api/v1/products/{sku}` |
| GET | `/oma-api/v1/projects/{project_id}/cart` |
| GET | `/oma-api/v1/carts/{cart_id}` |
| POST | `/oma-api/v1/carts/sync` |
| POST | `/oma-api/v1/carts/{cart_id}/quotes` |
| GET | `/oma-api/v1/quotes/{quote_id}` |
| POST | `/oma-api/v1/checkout-sessions` |
| GET | `/oma-api/v1/checkout-sessions/{checkout_id}` |
| POST | `/oma-api/v1/checkout-sessions/{checkout_id}/pay` |
| GET | `/oma-api/v1/orders/{order_id}` |
| POST | `/oma-api/v1/demo/orders/{order_id}/events` |

Данные: `public.oma_components` для app, `oma_shop.oma_components` для магазина.
При пустой oma_shop переносится legacy commerce из public. Старые fixture SKU доступны для
уже сохранённых проектов, новый поиск берётся из oma-site. Каталог читается из JSON при запуске;
обновление файла требует пересборки образа. Состояние корзин/заказов сохраняется в PostgreSQL
после успешных мутаций, перед отправкой ответа. `GET /healthz` магазина проверяет процесс;
Compose ждёт его перед запуском app. Для полного бэкапа БД используйте pg_dump из README.

Витрина — публичное демо без торговых учётных записей. `OMA_SERVICE_SECRET` должен быть одинаковым
у app/shop; он передаётся только сервером, не браузеру. Выставлять такое демо в production без
нормальной авторизации и изоляции корзин нельзя. Оплата всегда демонстрационная.

Нативный магазин:

```sh
MOCKOMA_ADDR=:8090 MOCKOMA_PRODUCTS_FILE=oma-site/data/products.json \
MOCKOMA_SITE_DIR=oma-site/site OMA_APP_BASE_URL=http://127.0.0.1:8088 \
MOCKOMA_STATE_PATH=var/shop-state.json go run ./cmd/mockoma
# в другом терминале с вашими настройками .env:
OMA_API_BASE_URL=http://127.0.0.1:8090 APP_ADDR=:8088 go run ./cmd/app
```

`DATABASE_URL` включает PostgreSQL у каждого процесса. Без него `MOCKOMA_STATE_PATH` сохраняет
магазин в локальный файл; без обеих переменных standalone-магазин работает в памяти.
Запускайте локальные тесты изолированно: `python3 scripts/shop_integration_smoke.py`.
