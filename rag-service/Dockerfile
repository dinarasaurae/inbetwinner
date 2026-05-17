# syntax=docker/dockerfile:1.7

FROM golang:1.26-alpine AS builder

ARG GOPROXY=https://proxy.golang.org|direct
ARG GOSUMDB=sum.golang.org
ENV GOPROXY=${GOPROXY} \
    GOSUMDB=${GOSUMDB}

RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

COPY rag-service/go.mod rag-service/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    for i in 1 2 3; do \
      go mod download && break; \
      if [ "$i" = "3" ]; then exit 1; fi; \
      sleep $((i * 5)); \
    done

COPY rag-service/ .

RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked \
    --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /rag-service ./cmd/server

FROM alpine:3.19
# poppler-utils provides pdftotext — handles CID fonts, Cyrillic, complex PDF layouts.
# Without it the service falls back to a basic pure-Go extractor (lower quality).
RUN apk --no-cache add ca-certificates curl tzdata poppler-utils
WORKDIR /app
COPY --from=builder /rag-service .
EXPOSE 3004
HEALTHCHECK --interval=10s --timeout=5s --retries=3 CMD curl -f http://localhost:3004/health || exit 1
USER nobody
CMD ["./rag-service"]
