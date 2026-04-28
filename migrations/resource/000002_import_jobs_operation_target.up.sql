BEGIN;

-- Split legacy single `type` (target only, import implied) into operation + target.
ALTER TABLE import_jobs
    ADD COLUMN operation_type TEXT NOT NULL DEFAULT 'import',
    ADD COLUMN target_type   TEXT;

UPDATE import_jobs SET target_type = type;

ALTER TABLE import_jobs
    ALTER COLUMN target_type SET NOT NULL,
    DROP COLUMN type;

ALTER TABLE import_jobs
    ALTER COLUMN operation_type DROP DEFAULT;

COMMIT;
