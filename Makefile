.PHONY: build test vet smoke run-app run-mock run-worker fmt

# Сборка всех приложений (app, mockoma, worker).
build:
	go build -o bin/ ./cmd/...

# Unit-тесты доменных модулей.
test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

# Сквозной smoke-тест: минимальный запуск без реального API ОМА.
smoke:
	./scripts/smoke.sh

run-app:
	go run ./cmd/app

run-mock:
	go run ./cmd/mockoma

# Мок ОМА с каталогом из oma.by и витриной на http://127.0.0.1:8090.
oma-site:
	MOCKOMA_PRODUCTS_FILE=oma-site/data/products.json MOCKOMA_SITE_DIR=oma-site/site go run ./cmd/mockoma

run-worker:
	go run ./cmd/worker

# Docker app + shop + PostgreSQL. Local secrets are read from .env by Compose.
.PHONY: run up down logs db run-local demo
run up:
	docker compose up --build -d --wait

down:
	docker compose down

logs:
	docker compose logs -f app

db:
	docker compose up -d --wait postgres

run-local:
	python3 scripts/run-local.py

demo:
	DATABASE_URL= AI_MODE=demo APP_ADDR=127.0.0.1:8088 OMA_STATE_PATH=./var/mvp-state.json MEDIA_STORAGE_PATH=./var/mvp-media go run ./cmd/app

.PHONY: smoke-shop
smoke-shop:
	python3 scripts/shop_integration_smoke.py
