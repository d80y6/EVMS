package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type SiteDiscoveryConfig struct {
	Enabled bool     `json:"enabled"`
	Methods []string `json:"methods"`
	Ports   []int    `json:"ports"`
	Subnets []string `json:"subnets"`
}

type Scheduler struct {
	db           *sqlx.DB
	orchestrator *ScanOrchestrator
	logger       *slog.Logger
	tickInterval time.Duration
}

func NewScheduler(db *sqlx.DB, orchestrator *ScanOrchestrator, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		db:           db,
		orchestrator: orchestrator,
		logger:       logger,
		tickInterval: 60 * time.Second,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.logger.Info("starting discovery scheduler", "interval", s.tickInterval)
	ticker := time.NewTicker(s.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.checkDueScans(ctx)
		}
	}
}

func (s *Scheduler) checkDueScans(ctx context.Context) {
	var sites []struct {
		ID              uuid.UUID `db:"id"`
		DiscoveryConfig *string   `db:"discovery_config"`
	}

	err := s.db.SelectContext(ctx, &sites, `
		SELECT id, discovery_config
		FROM sites
		WHERE discovery_config->>'enabled' = 'true'
	`)
	if err != nil {
		s.logger.Error("scheduler: failed to query sites", "error", err)
		return
	}

	for _, site := range sites {
		if site.DiscoveryConfig == nil || *site.DiscoveryConfig == "" {
			continue
		}
		cfg := SiteDiscoveryConfig{
			Enabled: true,
			Methods: []string{"ws-discovery", "ip-range"},
			Ports:   []int{80, 554, 8080},
		}

		_, err := s.orchestrator.StartScan(ctx, ScanRequest{
			SiteID:  site.ID,
			Methods: cfg.Methods,
			Subnets: cfg.Subnets,
			Ports:   cfg.Ports,
		})
		if err != nil {
			s.logger.Error("scheduler: failed to start scan", "site_id", site.ID, "error", err)
		} else {
			s.logger.Info("scheduler: started periodic scan", "site_id", site.ID)
		}
	}
}
