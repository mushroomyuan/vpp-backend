-- Tenant-scoped SubmitTask idempotency.
-- Applied against the dispatch database (see 50-dispatch-db.sh).
-- Keep in sync with migrations/dispatch/000002_task_idempotency.up.sql.

\c dispatch

ALTER TABLE dispatch_tasks
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_tasks_tenant_idempotency
    ON dispatch_tasks (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
