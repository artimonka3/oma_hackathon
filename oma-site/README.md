# oma-site — мок интеграции с каталогом ОМА

Отдельная папка с каталогом, собранным из публичных страниц oma.by, и витриной,
которая работает через контракты `/oma-api/v1` проекта. Служит для демонстрации
«как будто интеграция с ОМА уже подключена». Реальные покупки не совершаются,
все ответы помечены `demo: true`.

## Состав

- `data/products.json` — 611 товаров в контракте `oma.Product` (sku, название,
  цена в minor units, категория, единицы, упаковка, остаток) плюс витринные
  поля: `brand`, `description`, `image_url`, `source_url`, `old_price_minor`.
- `data/categories.json` — категории проекта с количеством товаров.
- `site/` — статичная витрина (HTML/CSS/JS без сборки): поиск, категории,
  карточка товара, демо-корзина с котировкой, checkout и демооплатой.
- `scraper/` — Python-скрипты сбора данных (без внешних пакетов, нужен только
  Python 3 и curl): `omafetch.py` (загрузчик с обходом cookie-проверки oma.by),
  `scrape.py` (обход каталога + разбор карточек), `build_catalog.py` (сборка
  финальных данных в контракт проекта).
- `raw/` — кэш HTML и сырой скрап (не коммитится, см. `.gitignore`).

## Запуск витрины вместе с API

```sh
make oma-site
```

Откроется `http://127.0.0.1:8090` — тот же процесс `cmd/mockoma` раздаёт и
витрину, и API с загруженным каталогом (переменные `MOCKOMA_PRODUCTS_FILE` и
`MOCKOMA_SITE_DIR`).

Без витрины (только API):

```sh
MOCKOMA_PRODUCTS_FILE=oma-site/data/products.json go run ./cmd/mockoma
```

Витрина отдельно от API — откройте `site/index.html` любым статик-сервером и
укажите базу API через `?api=http://127.0.0.1:8090/oma-api/v1` (CORS на API
уже разрешён). Выбор запоминается в `localStorage`.

## API

Эндпоинты симулятора (`cmd/mockoma`), теперь с каталогом из oma.by:

- `GET /healthz`
- `GET /oma-api/v1/products?q=&category=` — поиск по названию/sku/категории
- `GET /oma-api/v1/products/{sku}`
- `GET /oma-api/v1/categories` — категории с количеством товаров
- `GET /oma-api/v1/purchase-history?account=`
- `POST /oma-api/v1/carts/sync`, `GET /oma-api/v1/carts/{id}`
- `POST /oma-api/v1/carts/{id}/quotes`
- `POST /oma-api/v1/checkout-sessions`, `POST /oma-api/v1/checkout-sessions/{id}/pay`
- `GET /oma-api/v1/orders/{id}`, `POST /oma-api/v1/demo/orders/{id}/events`

## Маппинг данных

- Категории oma.by приведены к словарю проекта: `paint`, `primer`, `coating`,
  `wallpaper`, `flooring`, `tiling`, `door`, `accessory`, `misc`
  (см. `LEAF_MAP` в `build_catalog.py`); исходная категория сохранена в
  `oma_category`.
- Цены — minor units (копейки BYN), наличие → `stock` (есть: 50, нет: 0),
  `demo: true` у всех товаров.
- Для расчёта «ОМА.Собери» заполнены `material`/`coverage` у красок
  (`paint`, 0.1 л/м²) и `material` у ламината/линолеума/плитки.
- Единицы приведены к `pcs/m2/l/kg/roll/pack`; объём упаковки извлечён из
  названия («10 л» → `pack_amount: 10`).

## Обновление каталога

```sh
python3 oma-site/scraper/scrape.py       # обход разделов + страницы товаров (кэш в raw/cache)
python3 oma-site/scraper/build_catalog.py
```

Разделы заданы списком `SEEDS` в `scrape.py`. Скрипт идёт вежливо: пауза между
запросами, кэш на диске, лимит количества товаров. Частота перезапусков
ограничена: данные предназначены для локального демо, не для публичного хостинга.
