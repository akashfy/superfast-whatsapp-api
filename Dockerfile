# Build Stage
FROM golang:1.26-alpine AS builder

# Install build dependencies for sqlite3
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build with static linking for sqlite3
RUN CGO_ENABLED=1 GOOS=linux go build -o bot main.go

# Run Stage
FROM alpine:latest

RUN apk add --no-cache ca-certificates sqlite

WORKDIR /app

COPY --from=builder /app/bot .

EXPOSE 3000

CMD ["./bot"]
