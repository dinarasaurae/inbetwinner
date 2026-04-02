FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY llm-service/go.mod llm-service/go.sum ./
RUN go mod download

COPY llm-service/ .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /llm-service ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates curl tzdata
WORKDIR /app
COPY --from=builder /llm-service .
EXPOSE 3005
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3005/health || exit 1
USER nobody
CMD ["./llm-service"]
