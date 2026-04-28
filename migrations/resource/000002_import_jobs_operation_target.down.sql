BEGIN;

ALTER TABLE import_jobs
    ADD COLUMN type TEXT;

UPDATE import_jobs SET type = target_type WHERE operation_type = 'import';

-- Best-effort rollback for non-import rows (schema did not preserve a single legacy enum).
UPDATE import_jobs SET type = 'resource' WHERE type IS NULL;

ALTER TABLE import_jobs
    ALTER COLUMN type SET NOT NULL,
    DROP COLUMN operation_type,
    DROP COLUMN target_type;

COMMIT;
