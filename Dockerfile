# Builder Stage
FROM golang:1.25.6-alpine3.23 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o rss-fetcher ./cmd/server

# Runner Stage
FROM alpine:3.23.3

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/rss-fetcher .

EXPOSE 9090

ENTRYPOINT ["./rss-fetcher"]
