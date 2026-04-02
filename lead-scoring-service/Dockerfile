FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY lead-scoring-service/go.mod lead-scoring-service/go.sum ./
RUN go mod download

COPY lead-scoring-service/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /lead-scoring-service ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata curl
WORKDIR /app
COPY --from=builder /lead-scoring-service .
EXPOSE 3006
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3006/health || exit 1
USER nobody
CMD ["./lead-scoring-service"]
