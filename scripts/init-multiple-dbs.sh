#!/bin/bash
# Creates extra PostgreSQL databases on first container boot.
# Runs automatically as part of the postgres docker-entrypoint.d sequence.
# Only executes when postgres_data volume is empty (first start).

set -e

# social_db — used by social-service
SOCIAL_USER="${SOCIAL_DB_USER:-social_user}"
SOCIAL_PASS="${SOCIAL_DB_PASSWORD:-social_password}"
SOCIAL_DB="${SOCIAL_DB_NAME:-social_db}"

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    -- Create social service user and database
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
EOSQL

echo "Initialized database: ${SOCIAL_DB} (user: ${SOCIAL_USER})"
