#!/usr/bin/env bash
# Creates the gateway database for the gateway service.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE gateway'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'gateway')\gexec
EOSQL

echo "gateway database ready."
