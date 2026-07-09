#!/usr/bin/env bash
# Creates the dispatch database for the dispatch service.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE dispatch'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'dispatch')\gexec
EOSQL

echo "dispatch database ready."
