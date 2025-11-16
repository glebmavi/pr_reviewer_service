# Сборка
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем приложение
# -o /app/server - выходной бинарник
# -ldflags "-w -s" - убирает отладочную информацию, уменьшая размер
# CGO_ENABLED=0 - статическая сборка без C-зависимостей
RUN CGO_ENABLED=0 go build -o /app/server -ldflags="-w -s" ./cmd/server/main.go

# Релиз
FROM alpine:latest

WORKDIR /app

COPY ./configs/config.yml /app/configs/config.yml

COPY --from=builder /app/server /app/server

EXPOSE 8080

# TODO MIGHT BE WRONG
ENTRYPOINT ["/app/server"]