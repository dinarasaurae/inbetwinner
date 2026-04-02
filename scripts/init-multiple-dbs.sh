#!/bin/bash
# Creates extra PostgreSQL databases on first container boot.
# Runs automatically as part of the postgres docker-entrypoint.d sequence.
# Only executes when postgres_data volume is empty (first start).

set -e

# social_db — used by social-service
SOCIAL_USER="${SOCIAL_DB_USER:-social_user}"
SOCIAL_PASS="${SOCIAL_DB_PASSWORD:-social_password}"
SOCIAL_DB="${SOCIAL_DB_NAME:-social_db}"

# rag_db — used by rag-service
RAG_USER="${RAG_DB_USER:-rag_user}"
RAG_PASS="${RAG_DB_PASSWORD:-rag_password}"
RAG_DB="${RAG_DB_NAME:-rag_db}"

# llm_db — used by llm-service
LLM_USER="${LLM_DB_USER:-llm_user}"
LLM_PASS="${LLM_DB_PASSWORD:-llm_password}"
LLM_DB="${LLM_DB_NAME:-llm_db}"

# agent_db — used by agent-service
AGENT_USER="${AGENT_DB_USER:-agent_user}"
AGENT_PASS="${AGENT_DB_PASSWORD:-agent_password}"
AGENT_DB="${AGENT_DB_NAME:-agent_db}"

# leads_db — used by lead-scoring-service
LEADS_USER="${LEADS_DB_USER:-leads_user}"
LEADS_PASS="${LEADS_DB_PASSWORD:-leads_password}"
LEADS_DB="${LEADS_DB_NAME:-leads_db}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- social-service
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${SOCIAL_USER}') THEN
            CREATE USER ${SOCIAL_USER} WITH PASSWORD '${SOCIAL_PASS}';
        END IF;
    END
    \$\$;
    SELECT 'CREATE DATABASE ${SOCIAL_DB} OWNER ${SOCIAL_USER}'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${SOCIAL_DB}')
    \gexec
    GRANT ALL PRIVILEGES ON DATABASE ${SOCIAL_DB} TO ${SOCIAL_USER};

    -- rag-service
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${RAG_USER}') THEN
            CREATE USER ${RAG_USER} WITH PASSWORD '${RAG_PASS}';
        END IF;
    END
    \$\$;
    SELECT 'CREATE DATABASE ${RAG_DB} OWNER ${RAG_USER}'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${RAG_DB}')
    \gexec
    GRANT ALL PRIVILEGES ON DATABASE ${RAG_DB} TO ${RAG_USER};

    -- llm-service
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${LLM_USER}') THEN
            CREATE USER ${LLM_USER} WITH PASSWORD '${LLM_PASS}';
        END IF;
    END
    \$\$;
    SELECT 'CREATE DATABASE ${LLM_DB} OWNER ${LLM_USER}'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${LLM_DB}')
    \gexec
    GRANT ALL PRIVILEGES ON DATABASE ${LLM_DB} TO ${LLM_USER};

    -- agent-service
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${AGENT_USER}') THEN
            CREATE USER ${AGENT_USER} WITH PASSWORD '${AGENT_PASS}';
        END IF;
    END
    \$\$;
    SELECT 'CREATE DATABASE ${AGENT_DB} OWNER ${AGENT_USER}'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${AGENT_DB}')
    \gexec
    GRANT ALL PRIVILEGES ON DATABASE ${AGENT_DB} TO ${AGENT_USER};

    -- lead-scoring-service
    DO \$\$
    BEGIN
        IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = '${LEADS_USER}') THEN
            CREATE USER ${LEADS_USER} WITH PASSWORD '${LEADS_PASS}';
        END IF;
    END
    \$\$;
    SELECT 'CREATE DATABASE ${LEADS_DB} OWNER ${LEADS_USER}'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${LEADS_DB}')
    \gexec
    GRANT ALL PRIVILEGES ON DATABASE ${LEADS_DB} TO ${LEADS_USER};
EOSQL

echo "Initialized databases: ${SOCIAL_DB}, ${RAG_DB}, ${LLM_DB}, ${AGENT_DB}, ${LEADS_DB}"
