-- ONVIF credential columns for cameras
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS onvif_username TEXT;
ALTER TABLE cameras ADD COLUMN IF NOT EXISTS onvif_password TEXT;
