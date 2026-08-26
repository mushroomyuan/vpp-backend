DROP INDEX IF EXISTS alarm_event_dedup_ingested_at_idx;
DROP TABLE IF EXISTS alarm_event_dedup;

DROP INDEX IF EXISTS alarms_open_fingerprint_uidx;
DROP INDEX IF EXISTS alarms_tenant_fingerprint_idx;
DROP INDEX IF EXISTS alarms_tenant_status_last_idx;
DROP TABLE IF EXISTS alarms;
