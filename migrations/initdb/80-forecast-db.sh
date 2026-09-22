#!/usr/bin/env bash
# Creates the forecast database for the forecast service.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE forecast'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'forecast')\gexec
EOSQL

echo "forecast database ready."
