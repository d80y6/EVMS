package main

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

type StorageTier int

const (
	TierHot  StorageTier = iota
	TierWarm
	TierCold
)

type TierConfig struct {
	HotPath       string        `json:"hot_path"`
	WarmPath      string        `json:"warm_path"`
	ColdPath      string        `json:"cold_path"`
	WarmDays      int           `json:"warm_days"`
	ColdDays      int           `json:"cold_days"`
	CheckInterval time.Duration `json:"check_interval"`
}

type TieringManager struct {
	config TierConfig
	logger *slog.Logger
	mu     sync.Mutex
	active map[string]bool
}

func NewTieringManager(config TierConfig, logger *slog.Logger) *TieringManager {
	if config.CheckInterval == 0 {
		config.CheckInterval = 1 * time.Hour
	}
	return &TieringManager{
		config: config,
		logger: logger,
		active: make(map[string]bool),
	}
}

func (m *TieringManager) Start(ctx context.Context) {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	m.tierSegments()

	for {
		select {
		case <-ticker.C:
			m.tierSegments()
		case <-ctx.Done():
			return
		}
	}
}

func (m *TieringManager) tierSegments() {
	if m.config.WarmPath == "" && m.config.ColdPath == "" {
		return
	}

	filepath.Walk(m.config.HotPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		age := time.Since(info.ModTime())
		m.mu.Lock()
		if m.active[path] {
			m.mu.Unlock()
			return nil
		}
		m.active[path] = true
		m.mu.Unlock()

		defer func() {
			m.mu.Lock()
			delete(m.active, path)
			m.mu.Unlock()
		}()

		if m.config.ColdDays > 0 && age.Hours() > float64(m.config.ColdDays*24) {
			m.moveToCold(path)
		} else if m.config.WarmDays > 0 && age.Hours() > float64(m.config.WarmDays*24) {
			m.moveToWarm(path)
		}

		return nil
	})
}

func (m *TieringManager) moveToWarm(src string) {
	rel, _ := filepath.Rel(m.config.HotPath, src)
	dst := filepath.Join(m.config.WarmPath, rel)

	os.MkdirAll(filepath.Dir(dst), 0755)

	if err := copyFile(src, dst); err != nil {
		m.logger.Error("failed to move to warm", "src", src, "error", err)
		return
	}
	os.Remove(src)
	m.logger.Info("tiered to warm", "file", rel)
}

func (m *TieringManager) moveToCold(src string) {
	rel, _ := filepath.Rel(m.config.HotPath, src)
	dst := filepath.Join(m.config.ColdPath, rel)

	os.MkdirAll(filepath.Dir(dst), 0755)

	cmd := exec.Command("aws", "s3", "cp", src, dst)
	if output, err := cmd.CombinedOutput(); err != nil {
		m.logger.Error("failed to move to cold (S3)", "src", src, "error", err, "output", string(output))
		return
	}
	os.Remove(src)
	m.logger.Info("tiered to cold (S3)", "file", rel)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
