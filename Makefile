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

run-worker:
	go run ./cmd/worker
