#!/usr/bin/env bash
# Creates the casdoor database for the Casdoor IdP (Phase 2 OIDC).
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE casdoor'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'casdoor')\gexec
EOSQL

echo "casdoor database ready."
