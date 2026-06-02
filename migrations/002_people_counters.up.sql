CREATE TABLE IF NOT EXISTS people_counters (
    camera_id UUID NOT NULL,
    zone_id TEXT NOT NULL,
    direction TEXT NOT NULL,
    bucket TIMESTAMPTZ NOT NULL,
    count INT NOT NULL DEFAULT 0,
    PRIMARY KEY (camera_id, zone_id, direction, bucket)
);

SELECT create_hypertable('people_counters', 'bucket', if_not_exists => true);

CREATE INDEX IF NOT EXISTS idx_people_counters_lookup
    ON people_counters(camera_id, zone_id, bucket DESC);
