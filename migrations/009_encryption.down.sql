-- 007: Rollback audit log hash chain columns
ALTER TABLE audit_logs DROP COLUMN IF EXISTS hash;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS previous_hash;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS details_text;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS resource_id_str;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS actor;
