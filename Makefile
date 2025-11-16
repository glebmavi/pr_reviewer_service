.PHONY: help generate lint build-docker run-docker stop-docker test

help:
	@echo "Доступные команды:"
	@echo "  generate    - Запустить oapi-codegen и sqlc для генерации Go-кода"
	@echo "  lint        - Запустить golangci-lint"
	@echo "  build-docker- Запустить docker-compose build"
	@echo "  up          - Запустить docker-compose up (сборка + запуск)"
	@echo "  down        - Остановить docker-compose"
	@echo "  test        - Запустить go test (пока не реализовано)"
	@echo "  test-e2e    - Запустить e2e тесты (пока не реализовано)"

# Генерирует Go-код из openapi.yaml и .sql
generate:
	@echo "==> Генерация API-клиента (oapi-codegen)..."
	go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@latest \
		-generate "types,chi-server,spec" \
		-package api \
		-o ./pkg/api/server.gen.go \
		./openapi.yml

	@echo "==> Генерация DB-слоя (sqlc)..."
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate

build-docker:
	docker-compose build

up:
	docker-compose up --build -d

down:
	docker-compose down

lint:
	@echo "==> Запуск линтера (golangci-lint)..."
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2 run ./... --config .golangci.yml

test:
	go test -v ./...

test-e2e:
	go test -v ./test/e2e/...