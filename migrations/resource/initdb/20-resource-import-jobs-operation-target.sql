ALTER TABLE import_jobs
    ADD COLUMN IF NOT EXISTS operation_type TEXT,
    ADD COLUMN IF NOT EXISTS target_type TEXT;

UPDATE import_jobs
SET
    operation_type = COALESCE(NULLIF(operation_type, ''), 'import'),
    target_type = COALESCE(NULLIF(target_type, ''), 'resource')
WHERE operation_type IS NULL
   OR target_type IS NULL
   OR operation_type = ''
   OR target_type = '';

ALTER TABLE import_jobs
    ALTER COLUMN operation_type SET NOT NULL,
    ALTER COLUMN target_type SET NOT NULL;
