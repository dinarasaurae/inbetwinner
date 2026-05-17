# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

ARG GOPROXY=https://proxy.golang.org|direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

RUN apk add --no-cache \
    git \
    ca-certificates \
    tzdata

WORKDIR /build

COPY ./shared-libs ./shared-libs

COPY ./inbetwin-api-gateway .

RUN sed -i 's|../shared-libs/jwt-go|./shared-libs/jwt-go|' go.mod

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    for i in 1 2 3; do \
      go mod download && break; \
      if [ "$i" = "3" ]; then exit 1; fi; \
      sleep $((i * 5)); \
    done

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o api-gateway \
    ./cmd/server

FROM alpine:latest

RUN apk --no-cache add \
    ca-certificates \
    tzdata \
    curl

RUN addgroup -g 1000 appuser && \
    adduser -D -u 1000 -G appuser appuser

WORKDIR /app

COPY --from=builder /build/api-gateway .

RUN chown appuser:appuser api-gateway

USER appuser

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

CMD ["./api-gateway"]
