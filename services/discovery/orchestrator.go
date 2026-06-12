package main

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

type ScanRequest struct {
	SiteID    *uuid.UUID
	Methods   []string
	Subnets   []string
	Ports     []int
	CreatedBy *uuid.UUID
}

type Store interface {
	CreateScan(ctx context.Context, scan *ScanRecord) error
	UpdateScanStatus(ctx context.Context, id uuid.UUID, status string, totalFound int, errMsg *string) error
	GetScan(ctx context.Context, id uuid.UUID) (*ScanRecord, error)
	GetScans(ctx context.Context, siteID *uuid.UUID, page, perPage int) ([]ScanRecord, int, error)
	InsertResult(ctx context.Context, result *ResultRecord) error
	GetResults(ctx context.Context, scanID uuid.UUID, page, perPage int, queryFilter string) ([]ResultRecord, int, error)
	MarkImported(ctx context.Context, resultIDs []uuid.UUID) error
	CheckAlreadyInDB(ctx context.Context, xaddr string) (bool, error)
	Close() error
}

type ScanOrchestrator struct {
	store       Store
	scanners    map[string]Scanner
	logger      *slog.Logger
	activeMu    sync.Mutex
	activeScans map[uuid.UUID]context.CancelFunc
}

func NewScanOrchestrator(store Store, scanners map[string]Scanner, logger *slog.Logger) *ScanOrchestrator {
	return &ScanOrchestrator{
		store:       store,
		scanners:    scanners,
		logger:      logger,
		activeScans: make(map[uuid.UUID]context.CancelFunc),
	}
}

func (o *ScanOrchestrator) StartScan(ctx context.Context, req ScanRequest) (*ScanRecord, error) {
	scan := &ScanRecord{
		ID:        uuid.New(),
		SiteID:    req.SiteID,
		Status:    "pending",
		Methods:   req.Methods,
		Subnets:   req.Subnets,
		Ports:     req.Ports,
		CreatedBy: req.CreatedBy,
	}

	if err := o.store.CreateScan(ctx, scan); err != nil {
		return nil, fmt.Errorf("failed to create scan record: %w", err)
	}

	scanCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	o.activeMu.Lock()
	o.activeScans[scan.ID] = cancel
	o.activeMu.Unlock()

	now := time.Now()
	scan.Status = "running"
	scan.StartedAt = &now
	if err := o.store.UpdateScanStatus(ctx, scan.ID, "running", 0, nil); err != nil {
		cancel()
		return nil, err
	}

	go o.executeScan(scanCtx, scan, req)

	return scan, nil
}

func (o *ScanOrchestrator) CancelScan(ctx context.Context, scanID uuid.UUID) error {
	o.activeMu.Lock()
	defer o.activeMu.Unlock()

	cancel, ok := o.activeScans[scanID]
	if !ok {
		return fmt.Errorf("scan %s is not active", scanID)
	}
	cancel()
	return o.store.UpdateScanStatus(ctx, scanID, "cancelled", 0, nil)
}

func (o *ScanOrchestrator) executeScan(ctx context.Context, scan *ScanRecord, req ScanRequest) {
	defer func() {
		o.activeMu.Lock()
		delete(o.activeScans, scan.ID)
		o.activeMu.Unlock()
	}()

	var found int
	resultCh := make(chan ScanResult, 100)
	var scannerWg sync.WaitGroup

	for _, method := range req.Methods {
		scanner, ok := o.scanners[method]
		if !ok {
			o.logger.Warn("unknown scanner method", "method", method)
			continue
		}
		for _, subnet := range req.Subnets {
			scannerWg.Add(1)
			go func(sc Scanner, subnet string) {
				defer scannerWg.Done()
				ch, err := sc.Scan(ctx, subnet, req.Ports, ScanOptions{Timeout: 5 * time.Second})
				if err != nil {
					o.logger.Error("scanner failed to start", "method", sc.Name(), "subnet", subnet, "error", err)
					return
				}
				for res := range ch {
					select {
					case <-ctx.Done():
						return
					case resultCh <- res:
					}
				}
			}(scanner, subnet)
		}
	}

	go func() {
		scannerWg.Wait()
		close(resultCh)
	}()

	seen := make(map[string]bool)
	for res := range resultCh {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if res.Error != nil {
			o.logger.Error("scan result error", "error", res.Error)
			continue
		}

		if seen[res.XAddr] {
			continue
		}
		seen[res.XAddr] = true

		alreadyInDB, _ := o.store.CheckAlreadyInDB(ctx, res.XAddr)

		record := &ResultRecord{
			ID:          uuid.New(),
			ScanID:      scan.ID,
			SiteID:      scan.SiteID,
			IPAddress:   res.IP,
			XAddr:       &res.XAddr,
			IsNew:       true,
			AlreadyInDB: alreadyInDB,
		}
		if res.Port > 0 {
			record.Port = &res.Port
		}
		if res.Manufacturer != "" {
			record.Manufacturer = &res.Manufacturer
		}
		if res.Model != "" {
			record.Model = &res.Model
		}
		if res.Firmware != "" {
			record.Firmware = &res.Firmware
		}
		if res.SerialNumber != "" {
			record.SerialNumber = &res.SerialNumber
		}
		if res.Hostname != "" {
			record.Hostname = &res.Hostname
		}
		if res.Capabilities != nil {
			caps := make(map[string]interface{})
			for k, v := range res.Capabilities {
				caps[k] = v
			}
			record.Capabilities = caps
		}

		if err := o.store.InsertResult(ctx, record); err != nil {
			o.logger.Error("failed to store result", "error", err)
			continue
		}
		found++
	}

	errStr := ""
	if ctx.Err() != nil && ctx.Err() != context.Canceled {
		errStr = ctx.Err().Error()
	}
	status := "completed"
	if errStr != "" {
		status = "failed"
	}
	o.store.UpdateScanStatus(ctx, scan.ID, status, found, &errStr)
	o.logger.Info("scan completed", "scan_id", scan.ID, "found", found, "status", status)
}
