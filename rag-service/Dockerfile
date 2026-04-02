FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY rag-service/go.mod rag-service/go.sum ./
RUN go mod download

COPY rag-service/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /rag-service ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates curl tzdata
WORKDIR /app
COPY --from=builder /rag-service .
EXPOSE 3004
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3004/health || exit 1
USER nobody
CMD ["./rag-service"]
