FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /llm-service ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /llm-service .
EXPOSE 3004
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3004/health || exit 1
USER nobody
CMD ["./llm-service"]
