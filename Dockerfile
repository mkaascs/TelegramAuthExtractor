FROM golang:1.26 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /service ./cmd/telegram-auth-extractor

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends android-tools-adb ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY --from=builder /service /service
COPY config /config

EXPOSE 8080
ENTRYPOINT ["/service"]