package main

import (
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"
)

type PeopleCounter struct {
	db *sqlx.DB
}

type ZoneCrossing struct {
	CameraID   string    `json:"camera_id"`
	ZoneID     string    `json:"zone_id"`
	Direction  string    `json:"direction"`
	Count      int       `json:"count"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time  `json:"window_end"`
}

func NewPeopleCounter(db *sqlx.DB) *PeopleCounter {
	return &PeopleCounter{db: db}
}

func (pc *PeopleCounter) RecordCrossing(cameraID, zoneID, direction string, t time.Time) error {
	_, err := pc.db.Exec(
		`INSERT INTO people_counters (camera_id, zone_id, direction, bucket, count)
		 VALUES ($1, $2, $3, date_trunc('hour', $4), 1)
		 ON CONFLICT (camera_id, zone_id, direction, bucket)
		 DO UPDATE SET count = people_counters.count + 1`,
		cameraID, zoneID, direction, t)
	return err
}

func (pc *PeopleCounter) GetCount(cameraID, zoneID string, start, end time.Time) (int, error) {
	var total int
	err := pc.db.Get(&total,
		`SELECT COALESCE(SUM(CASE WHEN direction='enter' THEN count ELSE -count END), 0)
		 FROM people_counters
		 WHERE camera_id=$1 AND zone_id=$2 AND bucket BETWEEN $3 AND $4`,
		cameraID, zoneID, start, end)
	return total, err
}

func (pc *PeopleCounter) GetHourlyBreakdown(cameraID string, date time.Time) ([]ZoneCrossing, error) {
	var results []ZoneCrossing
	err := pc.db.Select(&results,
		`SELECT camera_id, zone_id, direction, bucket as window_start,
		        bucket + interval '1 hour' as window_end, count
		 FROM people_counters
		 WHERE camera_id=$1 AND bucket >= $2 AND bucket < $3
		 ORDER BY bucket`,
		cameraID, date, date.Add(24*time.Hour))
	return results, err
}
