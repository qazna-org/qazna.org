# syntax=docker/dockerfile:1

# ───────── Build stage ─────────
FROM golang:1.24-alpine AS builder

ARG VERSION="dev"
ARG COMMIT="local"

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Статически линкуем, чтобы рантайм был на чистом alpine без CGO
ENV CGO_ENABLED=0
RUN go build -ldflags "-X main.version=${VERSION} -X main.commit=${COMMIT}" -o /bin/qazna-api ./cmd/api

# ───────── Runtime stage ─────────
FROM alpine:3.20

WORKDIR /app
COPY --from=builder /bin/qazna-api /usr/local/bin/qazna-api
COPY --from=builder /src/web /app/web

# Простое здоровье: /healthz
HEALTHCHECK --interval=10s --timeout=3s --retries=10 CMD wget -qO- http://localhost:8080/healthz || exit 1

EXPOSE 8080 9090
ENTRYPOINT ["qazna-api"]
