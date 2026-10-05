DROP INDEX IF EXISTS uq_dispatch_tasks_tenant_idempotency;

ALTER TABLE dispatch_tasks
    DROP COLUMN IF EXISTS idempotency_key;
