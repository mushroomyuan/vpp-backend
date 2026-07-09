DROP INDEX IF EXISTS idx_control_commands_timeout_scan;
DROP INDEX IF EXISTS idx_control_commands_action_id;
DROP TABLE IF EXISTS control_commands;

DROP INDEX IF EXISTS idx_dispatch_actions_task_id;
DROP TABLE IF EXISTS dispatch_actions;

DROP INDEX IF EXISTS idx_dispatch_tasks_tenant_status;
DROP TABLE IF EXISTS dispatch_tasks;
