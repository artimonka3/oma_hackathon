# API рабочего MVP

Все запросы к тому же origin, что и сайт. JSON: `Content-Type: application/json`.
Авторизованные маршруты требуют `Authorization: Bearer <token>`.
Конверт ошибки: `{code, message, details, request_id}`. Денежные суммы — целые копейки BYN,
геометрия — миллиметры. Не считайте суммы повторно в браузере.

## Состояние и конфигурация

| Метод и путь | Назначение |
|---|---|
| GET /healthz | Живость процесса |
| GET /readyz | Готовность, включая Ping PostgreSQL |
| GET /api/v1/config | Публичные ai_mode/ai_model, storage, shop_connected; без ключа |
| POST /api/v1/demo/session | `{participant:"client"}` или `foreman_demo_01..03`; только DEMO_MODE |
| GET /api/v1/projects | Доступные текущему участнику проекты |
| POST /api/v1/projects | `{goal, execution_mode:"self"}`; опциональный budget_minor |
| GET /api/v1/projects/{id}/application | Проект, комнаты, остатки, варианты, спецификация, корзина, проверки, план, отчёты, заказ и события |
| PATCH /api/v1/projects/{id} | Изменение goal и других полей с expected_version |
| GET /api/v1/catalog?q= | Демонстрационный каталог |

## Подбор

```json
PUT /api/v1/projects/{id}/room
{"name":"Гостиная","length_mm":5000,"width_mm":4000,"height_mm":2700,"dimensions_confirmed":true}
```

```json
PUT /api/v1/projects/{id}/inventory
{"items":[{"sku":"PAINT-MATT-WHITE","amount":"4","unit":"l"}]}
```

PUT inventory заменяет подтверждённый список. Пустой массив означает отсутствие остатков.
Каждый элемент валидируется до замены: SKU существует, единица совпадает, количество положительное.
API истории `/inventory-suggestions` создаёт только suggested-партии и не подтверждает их автоматически.

| Метод и путь | Назначение |
|---|---|
| POST /api/v1/projects/{id}/captures | multipart: `file`, `kind=photo`; файл до 16 МБ на сервере, UI до 10 МБ |
| GET /api/v1/media/{media_id} | Приватное чтение файла, требуется доступ к его проекту |
| POST /api/v1/projects/{id}/design-jobs | Создать задание подбора |
| GET /api/v1/jobs/{job_id} | state queued/running/succeeded/failed, result/error; доступ по проекту |
| POST /api/v1/projects/{id}/design-selections | `{variant_id}` |
| POST /api/v1/projects/{id}/specifications | Рассчитать выбранные материалы по подтверждённым размерам |
| POST /api/v1/projects/{id}/cart-sync | Заменить связанные проектные строки без дублирования |
| PATCH /api/v1/projects/{id}/cart | `{sku,packs}`; нельзя опуститься ниже расчётного минимума |

## Прораб, оформление и работы

| Метод и путь | Тело/назначение |
|---|---|
| GET /api/v1/projects/{id}/analysis | Виды работ и соответствующие специалисты |
| PUT /api/v1/projects/{id}/foreman | `{foreman_id}`; пустая строка — самостоятельный режим |
| POST /api/v1/projects/{id}/reviews | `{foreman_id,scope:"project"}` |
| POST /api/v1/reviews/{id}/decisions | `{action:"approve_current",comment}` либо request_info/visit/decline |
| POST /api/v1/projects/{id}/checkout | `{confirmed:true}`; вернуть checkout и quote |
| POST /api/v1/projects/{id}/pay | Демооплата подтверждённого checkout; повтор возвращает тот же заказ |
| POST /api/v1/projects/{id}/demo-delivery | Смоделировать доставку |
| POST /api/v1/projects/{id}/default-plan | Подготовить двухэтапный план для выбранного прораба |
| POST /api/v1/work-plans/{id}/confirmation | Подтвердить план владельцем |
| POST /api/v1/stages/{id}/start | Начать этап назначенным прорабом после доставки и предыдущей приёмки |
| POST /api/v1/stages/{id}/reports | `{comment,items,photo_ids}` |
| POST /api/v1/stages/{id}/decisions | `{expected_version,report_id,decision,acceptance_confirmed,comment}` |
| POST /api/v1/projects/{id}/finish | Завершить при 100% принятого обязательного объёма |

Решения по этапу: accept или request_changes. При accept нужно явное acceptance_confirmed=true;
при request_changes обязателен комментарий. Изменённая версия отчёта даёт 409.

Покупка проверяет актуальную спецификацию, подтверждённые размеры, текущий состав
и решение выбранного прораба, если выбран foreman-режим. Самостоятельная покупка не требует прораба.
После заказа состав защищён от редактирования; новый объём оформляется отдельным проектом.

## Основные ошибки

- 400 INVALID_INPUT — некорректный ввод или отсутствие явного подтверждения.
- 403 FORBIDDEN — чужой проект, job, фото или недопустимая роль.
- 409 VERSION_CONFLICT / REPORT_CHANGED / INVALID_TRANSITION / CART_CHANGED — устаревшая версия/переход.
- 422 NEEDS_DATA / REVIEW_REQUIRED — не хватает вводных либо актуальной проверки.
- 503 INTEGRATION_UNAVAILABLE — магазин недоступен.
- 501 NOT_IMPLEMENTED — оставшиеся контракты посредника/сложных изменений; в рабочем UI их нет.

Внутренние `/internal/v1/oma/*` предназначены для сервисов магазина, не для браузера.
Ключ AI и сервисный секрет никогда не передаются в клиентские запросы.

Витрина клона и её API доступны через `/shop/`, без передачи сервисного ключа в браузер. Контракт торговли: [SHOP.md](SHOP.md). AR использует Android intent в интерфейсе; серверных запросов сканирования нет.
