FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /agent-service ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /agent-service .
EXPOSE 3005
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3005/health || exit 1
USER nobody
CMD ["./agent-service"]
