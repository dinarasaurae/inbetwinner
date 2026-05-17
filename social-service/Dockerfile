# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

ARG GOPROXY=https://proxy.golang.org|direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY social-service/go.mod social-service/go.sum ./

COPY shared-libs/ ../shared-libs/

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    for i in 1 2 3; do \
      go mod download && break; \
      if [ "$i" = "3" ]; then exit 1; fi; \
      sleep $((i * 5)); \
    done

COPY social-service/ .

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o social-service ./cmd/server

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
