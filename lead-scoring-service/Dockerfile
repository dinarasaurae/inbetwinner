# syntax=docker/dockerfile:1.7

FROM golang:1.22-alpine AS builder

ARG GOPROXY=https://proxy.golang.org|direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY lead-scoring-service/go.mod lead-scoring-service/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    for i in 1 2 3; do \
      go mod download && break; \
      if [ "$i" = "3" ]; then exit 1; fi; \
      sleep $((i * 5)); \
    done

COPY lead-scoring-service/ .

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /lead-scoring-service ./cmd/server

FROM alpine:3.19
RUN apk --no-cache add ca-certificates tzdata curl
WORKDIR /app
COPY --from=builder /lead-scoring-service .
EXPOSE 3006
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3006/health || exit 1
USER nobody
CMD ["./lead-scoring-service"]
