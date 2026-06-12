package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/dam-vms/dam/pkg/common"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

type ScanRecord struct {
	ID          uuid.UUID  `db:"id" json:"id"`
	SiteID      *uuid.UUID `db:"site_id" json:"site_id"`
	Status      string     `db:"status" json:"status"`
	Methods     []string   `db:"methods" json:"methods"`
	Subnets     []string   `db:"subnets" json:"subnets"`
	Ports       []int      `db:"ports" json:"ports"`
	TotalFound  int        `db:"total_found" json:"total_found"`
	Error       *string    `db:"error" json:"error,omitempty"`
	StartedAt   *time.Time `db:"started_at" json:"started_at"`
	CompletedAt *time.Time `db:"completed_at" json:"completed_at"`
	CreatedBy   *uuid.UUID `db:"created_by" json:"created_by"`
	CreatedAt   time.Time  `db:"created_at" json:"created_at"`
}

type ResultRecord struct {
	ID           uuid.UUID              `db:"id" json:"id"`
	ScanID       uuid.UUID              `db:"scan_id" json:"scan_id"`
	SiteID       *uuid.UUID             `db:"site_id" json:"site_id"`
	IPAddress    string                 `db:"ip_address" json:"ip_address"`
	Port         *int                   `db:"port" json:"port"`
	XAddr        *string                `db:"xaddr" json:"xaddr"`
	Manufacturer *string                `db:"manufacturer" json:"manufacturer"`
	Model        *string                `db:"model" json:"model"`
	Firmware     *string                `db:"firmware" json:"firmware"`
	SerialNumber *string                `db:"serial_number" json:"serial_number"`
	Hostname     *string                `db:"hostname" json:"hostname"`
	Capabilities map[string]interface{} `db:"capabilities" json:"capabilities"`
	OnvifData    map[string]interface{} `db:"onvif_data" json:"onvif_data,omitempty"`
	IsNew        bool                   `db:"is_new" json:"is_new"`
	AlreadyInDB  bool                   `db:"already_in_db" json:"already_in_db"`
	Imported     bool                   `db:"imported" json:"imported"`
	ImportedAt   *time.Time             `db:"imported_at" json:"imported_at"`
	CreatedAt    time.Time              `db:"created_at" json:"created_at"`
}

type ResultStore struct {
	db     *common.CircuitBreakerDB
	rawDB  *sqlx.DB
	logger *slog.Logger
}

func NewResultStore(db *sqlx.DB, logger *slog.Logger) *ResultStore {
	cb := common.NewDBCircuitBreaker("discovery-store")
	return &ResultStore{
		db:     common.WrapDB(db, cb),
		rawDB:  db,
		logger: logger,
	}
}

func (s *ResultStore) CreateScan(ctx context.Context, scan *ScanRecord) error {
	query := `INSERT INTO discovery_scans (id, site_id, status, methods, subnets, ports, created_by)
              VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := s.db.ExecContext(ctx, query,
		scan.ID, scan.SiteID, scan.Status,
		pq.Array(scan.Methods), pq.Array(scan.Subnets), pq.Array(scan.Ports),
		scan.CreatedBy)
	return err
}

func (s *ResultStore) UpdateScanStatus(ctx context.Context, id uuid.UUID, status string, totalFound int, errMsg *string) error {
	now := time.Now()
	query := `UPDATE discovery_scans SET status=$1, total_found=$2, error=$3, completed_at=$4 WHERE id=$5`
	_, err := s.db.ExecContext(ctx, query, status, totalFound, errMsg, now, id)
	return err
}

func (s *ResultStore) GetScan(ctx context.Context, id uuid.UUID) (*ScanRecord, error) {
	query := `SELECT * FROM discovery_scans WHERE id=$1`
	var scan ScanRecord
	err := s.db.GetContext(ctx, &scan, query, id)
	if err != nil {
		return nil, err
	}
	return &scan, nil
}

func (s *ResultStore) GetScans(ctx context.Context, siteID *uuid.UUID, page, perPage int) ([]ScanRecord, int, error) {
	where := ""
	args := []interface{}{}
	if siteID != nil {
		where = "WHERE site_id = $1"
		args = append(args, *siteID)
	}
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM discovery_scans %s", where)
	if err := s.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * perPage
	query := fmt.Sprintf("SELECT * FROM discovery_scans %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, len(args)+1, len(args)+2)
	args = append(args, perPage, offset)
	var scans []ScanRecord
	if err := s.db.SelectContext(ctx, &scans, query, args...); err != nil {
		return nil, 0, err
	}
	return scans, total, nil
}

func (s *ResultStore) InsertResult(ctx context.Context, result *ResultRecord) error {
	query := `INSERT INTO discovery_results
        (id, scan_id, site_id, ip_address, port, xaddr, manufacturer, model, firmware,
         serial_number, hostname, capabilities, onvif_data, is_new, already_in_db)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`
	capsJSON := mapToJSON(result.Capabilities)
	onvifJSON := mapToJSON(result.OnvifData)
	_, err := s.db.ExecContext(ctx, query,
		result.ID, result.ScanID, result.SiteID, result.IPAddress, result.Port,
		result.XAddr, result.Manufacturer, result.Model, result.Firmware,
		result.SerialNumber, result.Hostname, capsJSON, onvifJSON,
		result.IsNew, result.AlreadyInDB)
	return err
}

func (s *ResultStore) GetResults(ctx context.Context, scanID uuid.UUID, page, perPage int, queryFilter string) ([]ResultRecord, int, error) {
	where := "WHERE scan_id = $1"
	args := []interface{}{scanID}
	argIdx := 2
	if queryFilter != "" {
		where += fmt.Sprintf(" AND (ip_address ILIKE $%d OR manufacturer ILIKE $%d OR model ILIKE $%d OR hostname ILIKE $%d)",
			argIdx, argIdx, argIdx, argIdx)
		args = append(args, "%"+queryFilter+"%")
		argIdx++
	}
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM discovery_results %s", where)
	if err := s.db.GetContext(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * perPage
	query := fmt.Sprintf("SELECT * FROM discovery_results %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d",
		where, argIdx, argIdx+1)
	args = append(args, perPage, offset)
	var results []ResultRecord
	if err := s.db.SelectContext(ctx, &results, query, args...); err != nil {
		return nil, 0, err
	}
	return results, total, nil
}

func (s *ResultStore) MarkImported(ctx context.Context, resultIDs []uuid.UUID) error {
	query := `UPDATE discovery_results SET imported=true, imported_at=NOW() WHERE id = ANY($1)`
	_, err := s.db.ExecContext(ctx, query, pq.Array(resultIDs))
	return err
}

func (s *ResultStore) CheckAlreadyInDB(ctx context.Context, xaddr string) (bool, error) {
	var count int
	err := s.db.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM cameras WHERE connection_url = $1`, xaddr)
	return count > 0, err
}

func (s *ResultStore) Close() error {
	return s.db.Close()
}

func mapToJSON(m map[string]interface{}) []byte {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}
