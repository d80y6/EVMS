ALTER TABLE cameras ADD COLUMN last_seen_online TIMESTAMPTZ;
ALTER TABLE cameras ADD COLUMN last_status_change TIMESTAMPTZ;
