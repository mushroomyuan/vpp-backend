-- Forecast service schema: bi-temporal prediction history.
-- Applied against the forecast database (see 80-forecast-db.sh).
-- Keep in sync with migrations/forecast/000001_init.up.sql.

\c forecast

CREATE TABLE IF NOT EXISTS forecast_history (
    id                UUID        PRIMARY KEY,
    tenant_id         TEXT        NOT NULL,
    cu_code           TEXT        NOT NULL,
    metric_name       TEXT        NOT NULL,
    generated_at      TIMESTAMPTZ NOT NULL,
    target_timestamp  TIMESTAMPTZ NOT NULL,
    predicted_value   DOUBLE PRECISION NOT NULL,
    algorithm_version TEXT        NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT forecast_history_unique
        UNIQUE (tenant_id, cu_code, metric_name, generated_at, target_timestamp)
);

CREATE INDEX IF NOT EXISTS forecast_history_query_idx
    ON forecast_history (tenant_id, cu_code, metric_name, target_timestamp DESC);
