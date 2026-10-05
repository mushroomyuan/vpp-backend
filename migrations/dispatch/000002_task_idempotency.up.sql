-- Tenant-scoped idempotency for SubmitTask. Empty keys stay nullable so
-- manual submits without a key can still create many tasks.
ALTER TABLE dispatch_tasks
    ADD COLUMN IF NOT EXISTS idempotency_key TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS uq_dispatch_tasks_tenant_idempotency
    ON dispatch_tasks (tenant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
