.PHONY: help generate lint build-docker up down test deps docker-check test-coverage ci

help:
	@echo "Доступные команды:"
	@echo "  generate      - Сгенерировать Go-код из openapi.yml и .sql файлов"
	@echo "  lint          - Запустить линтер golangci-lint"
	@echo "  build-docker  - Собрать docker-образы с помощью docker-compose"
	@echo "  up            - Собрать и запустить docker-контейнеры"
	@echo "  down          - Остановить docker-контейнеры"
	@echo "  deps          - Загрузить Go зависимости"
	@echo "  docker-check  - Проверить, запущен ли Docker"
	@echo "  test          - Запустить E2E тесты"
	@echo "  test-coverage - Запустить E2E тесты с генерацией отчета о покрытии"
	@echo "  ci            - Выполнить шаги CI: загрузка зависимостей и запуск тестов"

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

build-docker-test:
	docker-compose -f docker-compose.test.yml build

up:
	docker-compose up --build -d

down:
	docker-compose down

lint:
	@echo "==> Запуск линтера (golangci-lint)..."
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.6.2 run ./... --config .golangci.yml

deps: ## Загрузка Go зависимостей
	go mod download
	go mod tidy

docker-check: ## Проверка, запущен ли Docker
	@docker info > /dev/null 2>&1 || (echo "Ошибка: Docker не запущен" && exit 1)
	@echo "✓ Docker запущен"

test: docker-check build-docker-test ## Запуск E2E тестов
	@echo "Запуск E2E тестов..."
	go test -v ./... -timeout 10m

test-coverage: docker-check build-docker-test ## Запуск E2E тестов с отчетом о покрытии
	@echo "Запуск E2E тестов с покрытием..."
	go test ./... -v -timeout 10m -coverprofile=coverage.out
	@echo "Генерация HTML отчета о покрытии..."
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Отчет о покрытии сгенерирован: coverage.html"

ci: deps test ## Запуск тестов как в CI окружении
	@echo "✓ CI тесты пройдены"