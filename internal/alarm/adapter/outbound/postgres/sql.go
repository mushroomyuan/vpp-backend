package postgres

// Dedup insert is the first CTE in both statements. Do not reorder: a crash
// after upsert-but-before-dedup would double-count on replay.
const soeIngestSQL = `
WITH ins_dedup AS (
  INSERT INTO alarm_event_dedup (tenant_id, event_id, alarm_id, ingested_at)
  VALUES (?, ?, ?::uuid, now())
  ON CONFLICT (tenant_id, event_id) DO NOTHING
  RETURNING tenant_id, event_id
),
upsert_alarm AS (
  INSERT INTO alarms (
    id, tenant_id, fingerprint, source, status, severity, rule_id,
    title, summary, source_ref, attributes, attributes_schema,
    count, first_occurred_at, last_occurred_at, last_event_id, version
  )
  SELECT
    ?::uuid, ?, ?, 'soe', 'open', ?, ?,
    ?, ?, ?, ?::jsonb, ?,
    1, ?, ?, ?, 1
  FROM ins_dedup
  ON CONFLICT (tenant_id, fingerprint) WHERE status <> 'closed'
  DO UPDATE SET
    count            = alarms.count + 1,
    last_event_id    = EXCLUDED.last_event_id,
    title            = EXCLUDED.title,
    summary          = EXCLUDED.summary,
    attributes       = CASE WHEN EXCLUDED.last_occurred_at >= alarms.last_occurred_at
                            THEN EXCLUDED.attributes ELSE alarms.attributes END,
    last_occurred_at = GREATEST(alarms.last_occurred_at, EXCLUDED.last_occurred_at),
    version          = alarms.version + 1
  RETURNING id
),
backfill AS (
  UPDATE alarm_event_dedup d
  SET alarm_id = u.id
  FROM upsert_alarm u
  JOIN ins_dedup i ON TRUE
  WHERE d.tenant_id = i.tenant_id
    AND d.event_id  = i.event_id
    AND d.alarm_id IS DISTINCT FROM u.id
  RETURNING d.event_id
)
SELECT
  (SELECT COUNT(*) FROM ins_dedup)::int AS dedup_inserted,
  (SELECT id::text FROM upsert_alarm)   AS alarm_id
`

const dispatchIngestSQL = `
WITH ins_dedup AS (
  INSERT INTO alarm_event_dedup (tenant_id, event_id, alarm_id, ingested_at)
  VALUES (?, ?, ?::uuid, now())
  ON CONFLICT (tenant_id, event_id) DO NOTHING
  RETURNING tenant_id, event_id
),
ins_alarm AS (
  INSERT INTO alarms (
    id, tenant_id, fingerprint, source, status, severity, rule_id,
    title, summary, source_ref, attributes, attributes_schema,
    count, first_occurred_at, last_occurred_at, last_event_id, version
  )
  SELECT
    ?::uuid, ?, ?, 'dispatch', 'open', ?, ?,
    ?, ?, ?, ?::jsonb, ?,
    1, ?, ?, ?, 1
  FROM ins_dedup
  RETURNING id
)
SELECT
  (SELECT COUNT(*) FROM ins_dedup)::int AS dedup_inserted,
  (SELECT id::text FROM ins_alarm)      AS alarm_id
`

const alarmSelectCols = `
  id::text AS id,
  tenant_id,
  fingerprint,
  source,
  status,
  severity,
  rule_id,
  title,
  summary,
  source_ref,
  attributes,
  attributes_schema,
  count,
  first_occurred_at,
  last_occurred_at,
  last_event_id,
  acknowledged_at,
  acknowledged_by,
  closed_at,
  closed_by,
  version
`

const findByIDSQL = `SELECT` + alarmSelectCols + ` FROM alarms WHERE tenant_id = ? AND id = ?::uuid`

const countOpenBySourceSQL = `
SELECT source, COUNT(*)::int AS n
FROM alarms
WHERE status <> 'closed'
GROUP BY source
`

const ackSQL = `
UPDATE alarms
SET status = 'acknowledged',
    acknowledged_by = ?,
    acknowledged_at = ?,
    version = version + 1
WHERE id = ?::uuid AND tenant_id = ? AND version = ? AND status = 'open'
RETURNING` + alarmSelectCols

const closeSQL = `
UPDATE alarms
SET status = 'closed',
    closed_by = ?,
    closed_at = ?,
    version = version + 1
WHERE id = ?::uuid AND tenant_id = ? AND version = ? AND status IN ('open', 'acknowledged')
RETURNING` + alarmSelectCols
