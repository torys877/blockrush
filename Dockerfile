FROM golang:1.23 AS builder

WORKDIR /app

COPY . .

RUN go mod tidy && go build -o blockrush

FROM debian:bookworm-slim

WORKDIR /app

RUN apt-get update && apt-get install -y libc6

COPY --from=builder /app/blockrush /app/blockrush

RUN mkdir -p /app/logs

CMD ["sh", "-c", "/app/blockrush --config=$CONFIG_PATH"]
