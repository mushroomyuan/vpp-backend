ALTER TABLE import_jobs
    DROP COLUMN IF EXISTS target_type,
    DROP COLUMN IF EXISTS operation_type;
