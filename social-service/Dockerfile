FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY social-service/go.mod social-service/go.sum ./

COPY shared-libs/ ../shared-libs/

RUN go mod download

COPY social-service/ .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o social-service ./cmd/server

FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata curl

RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /root/

COPY --from=builder /app/social-service .
COPY --from=builder /app/migrations ./migrations

RUN chown -R appuser:appgroup /root

USER appuser

EXPOSE 3002

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD curl -f http://localhost:3002/health || exit 1

CMD ["./social-service"]
