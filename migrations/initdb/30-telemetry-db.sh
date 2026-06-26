#!/usr/bin/env bash
# Creates the telemetry database and enables the TimescaleDB extension.
# The resource database (POSTGRES_DB) is already created by the Docker entrypoint;
# this script only needs to handle the second database.
set -e

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" <<-EOSQL
    SELECT 'CREATE DATABASE telemetry'
    WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'telemetry')\gexec
EOSQL

psql -v ON_ERROR_STOP=1 --username "$POSTGRES_USER" --dbname "telemetry" <<-EOSQL
    CREATE EXTENSION IF NOT EXISTS timescaledb;
EOSQL

echo "telemetry database ready."
