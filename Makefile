.PHONY: help build run test lint generate

# Команда для генерации oapi и sqlc
generate:
    oapi-codegen -generate "types,chi-server,spec" -package api -o pkg/api/server.gen.go openapi.yaml
    sqlc generate -f sqlc.yaml

# Сборка бинарника
build:
    go build -o ./bin/server ./cmd/server/main.go

# Запуск локально (требует запущенного postgres)
run:
    go run ./cmd/server/main.go

# Запуск всего стека
up:
    docker-compose up --build

# Остановка
down:
    docker-compose down

# Запуск линтера
lint:
    golangci-lint run ./...

# Запуск тестов
test:
    go test -v ./...

# Запуск E2E тестов
test-e2e:
    go test -v ./test/e2e/...