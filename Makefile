
# ──────────────────────────────────────────────────────────────────────────────
# inBeTwin — top-level Makefile
# ──────────────────────────────────────────────────────────────────────────────

.PHONY: help up down logs test-integration test-unit test-provider clean

# ── Local stack ───────────────────────────────────────────────────────────────

help:
	@echo ""
	@echo "  make up                  — start full docker compose stack"
	@echo "  make down                — stop and remove containers"
	@echo "  make logs                — tail logs from all services"
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
