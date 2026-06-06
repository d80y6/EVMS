-- Migration: 010_discovery.down.sql

DROP TABLE IF EXISTS discovery_results;
DROP TABLE IF EXISTS discovery_scans;
ALTER TABLE sites DROP COLUMN IF EXISTS discovery_config;
