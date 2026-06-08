
# ──────────────────────────────────────────────────────────────────────────────
# inBeTwin — top-level Makefile
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: help up down logs monitoring-up monitoring-down monitoring-logs smoke-local smoke-prod test-integration test-unit test-provider clean

# ── Local stack ───────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "  make up                  — start full docker compose stack"
	@echo "  make down                — stop and remove containers"
	@echo "  make logs                — tail logs from all services"
	@echo "  make monitoring-up       — start stack with Prometheus + Grafana"
	@echo "  make monitoring-down     — stop stack with monitoring overlay"
	@echo "  make monitoring-logs     — tail monitoring logs"
	@echo "  make smoke-local         — run a gentle k6 smoke test against local Docker"
	@echo "  make smoke-prod          — run the same k6 smoke test against production"
	@echo ""
	@echo "  make test-unit           — run pure unit tests (no DB, no network)"
	@echo "  make test-provider       — run LLM provider unit tests in llm-service"
	@echo "  make test-integration    — spin up test Postgres, run e2e integration"
	@echo "                             tests, tear down.  Requires Docker."
	@echo ""

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

monitoring-up:
	docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d --build

monitoring-down:
	docker compose -f docker-compose.yml -f docker-compose.monitoring.yml down

monitoring-logs:
	docker compose -f docker-compose.yml -f docker-compose.monitoring.yml logs -f prometheus grafana blackbox-exporter cadvisor postgres-exporter redis-exporter rabbitmq

# ── k6 smoke load test ───────────────────────────────────────────────────────

K6_IMAGE ?= grafana/k6:latest
K6_BASE_URL ?= http://host.docker.internal
K6_PROD_BASE_URL ?= https://inbetwin.ru
K6_VUS ?= 2
K6_DURATION ?= 3m
K6_RAMP_UP ?= 30s
K6_RAMP_DOWN ?= 30s
K6_P95_MS ?= 300
K6_ERROR_RATE ?= 0.01
K6_SUCCESS_RATE ?= 0.99
K6_TEST_EMAIL ?= nfr-smoke@example.com
K6_TEST_PASSWORD ?= nfr-smoke-password

smoke-local:
	docker run --rm --add-host=host.docker.internal:host-gateway \
		-e BASE_URL="$(K6_BASE_URL)" \
		-e K6_AUTO_REGISTER="true" \
		-e TEST_EMAIL="$(K6_TEST_EMAIL)" \
		-e TEST_PASSWORD="$(K6_TEST_PASSWORD)" \
		-e K6_VUS="$(K6_VUS)" \
		-e K6_DURATION="$(K6_DURATION)" \
		-e K6_RAMP_UP="$(K6_RAMP_UP)" \
		-e K6_RAMP_DOWN="$(K6_RAMP_DOWN)" \
		-e K6_P95_MS="$(K6_P95_MS)" \
		-e K6_ERROR_RATE="$(K6_ERROR_RATE)" \
		-e K6_SUCCESS_RATE="$(K6_SUCCESS_RATE)" \
		-v "$(CURDIR)/tests/k6:/scripts:ro" \
		$(K6_IMAGE) run /scripts/smoke.js

smoke-prod:
	@if [ "$(K6_TEST_EMAIL)" = "nfr-smoke@example.com" ]; then \
		echo "Set K6_TEST_EMAIL and K6_TEST_PASSWORD for an existing production test user."; \
		exit 1; \
	fi
	docker run --rm \
		-e BASE_URL="$(K6_PROD_BASE_URL)" \
		-e K6_AUTO_REGISTER="false" \
		-e TEST_EMAIL="$(K6_TEST_EMAIL)" \
		-e TEST_PASSWORD="$(K6_TEST_PASSWORD)" \
		-e K6_VUS="$(K6_VUS)" \
		-e K6_DURATION="$(K6_DURATION)" \
		-e K6_RAMP_UP="$(K6_RAMP_UP)" \
		-e K6_RAMP_DOWN="$(K6_RAMP_DOWN)" \
		-e K6_P95_MS="$(K6_P95_MS)" \
		-e K6_ERROR_RATE="$(K6_ERROR_RATE)" \
		-e K6_SUCCESS_RATE="$(K6_SUCCESS_RATE)" \
		-v "$(CURDIR)/tests/k6:/scripts:ro" \
		$(K6_IMAGE) run /scripts/smoke.js

# ── Unit tests (no Docker required) ───────────────────────────────────────────

test-unit:
	cd social-service  && go test ./...
	cd llm-service     && go test ./...
	cd rag-service     && go test ./...

test-provider:
	cd llm-service && go test -v ./internal/services/llmprovider/... -run .

# ── Integration tests ─────────────────────────────────────────────────────────
# Starts a dedicated test Postgres on port 5433, waits until healthy,
# exports INTEGRATION_TEST_DB, runs all *integration-tagged* tests in
# social-service, then tears everything down — even on failure.
#
# Prerequisites: docker, go 1.22+
# ─────────────────────────────────────────────────────────────────────────────

TEST_DSN := postgres://social_user:social_password@localhost:5433/social_db?sslmode=disable

test-integration:
	@echo "── Starting test Postgres (port 5433) ────────────────────────────────────"
	docker compose -f docker-compose.test.yml up -d
	@echo "── Waiting for postgres-test to be healthy ───────────────────────────────"
	@until docker inspect --format='{{.State.Health.Status}}' inbetwin-postgres-test 2>/dev/null | grep -q healthy; do \
	    printf "."; sleep 1; \
	done; echo " ready"
	@echo "── Running social-service integration tests ─────────────────────────────"
	cd social-service && \
	    INTEGRATION_TEST_DB="$(TEST_DSN)" \
	    go test -tags=integration -v -timeout=120s ./internal/services/... \
	    ; EXIT=$$?; \
	    echo "── Tearing down test Postgres ───────────────────────────────────────────"; \
	    docker compose -f ../docker-compose.test.yml down -v; \
	    exit $$EXIT

clean:
	docker compose -f docker-compose.test.yml down -v 2>/dev/null || true
	docker compose down -v 2>/dev/null || true
