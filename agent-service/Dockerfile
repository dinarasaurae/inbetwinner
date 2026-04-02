FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY agent-service/go.mod agent-service/go.sum ./
RUN go mod download

COPY agent-service/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /agent-service ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates curl tzdata
WORKDIR /app
COPY --from=builder /agent-service .
EXPOSE 3003
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3003/health || exit 1
USER nobody
CMD ["./agent-service"]
