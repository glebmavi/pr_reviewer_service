# Сборка
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git build-base

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ENV CGO_ENABLED=0
RUN go build \
    -trimpath \
    -ldflags="-w -s" \
    -o server ./cmd/server/main.go

# Релиз
FROM scratch

WORKDIR /app

COPY --from=builder /app/server /app/server

EXPOSE 8080

ENTRYPOINT ["/app/server"]