#!/usr/bin/env bash
# Creates the decision database for the decision service.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE decision'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'decision')\gexec
EOSQL

echo "decision database ready."
