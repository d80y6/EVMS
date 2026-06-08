-- 007: Credential encryption and audit log hash chain support
-- ENCRYPTION_KEY env var must be 64-char hex (32 bytes, AES-256)
-- Generate with: openssl rand -hex 32
-- Encryption is handled at the application layer; no schema changes for cameras.
-- These columns extend the audit_logs table for tamper-evident hash chains.

ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS actor TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS resource_id_str TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS details_text TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS previous_hash TEXT;
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS hash TEXT;
