package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	S3Retries     int           `json:"s3_retries"`
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
	if config.S3Retries == 0 {
		config.S3Retries = 3
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

func (m *TieringManager) s3Upload(src, dst string) error {
	if isS3Path(dst) {
		checksum, err := fileChecksum(src)
		if err != nil {
			return fmt.Errorf("checksum failed: %w", err)
		}

		cmd := exec.Command("aws", "s3", "cp", src, dst, "--metadata", fmt.Sprintf("sha256=%s", checksum))
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("s3 cp failed: %s: %w", string(output), err)
		}

		if err := m.verifyS3Upload(src, dst, checksum); err != nil {
			return fmt.Errorf("s3 verification failed: %w", err)
		}
	} else {
		os.MkdirAll(filepath.Dir(dst), 0755)
		if err := copyFile(src, dst); err != nil {
			return err
		}
	}
	return nil
}

func (m *TieringManager) verifyS3Upload(src, dst, expectedChecksum string) error {
	lsCmd := exec.Command("aws", "s3", "ls", dst)
	if output, err := lsCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("s3 ls failed for verification: %s: %w", string(output), err)
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("failed to stat source for verification: %w", err)
	}

	headCmd := exec.Command("aws", "s3api", "head-object", "--bucket", s3BucketFromPath(dst), "--key", s3KeyFromPath(dst))
	if output, err := headCmd.CombinedOutput(); err != nil {
		m.logger.Warn("s3 head-object verification unavailable", "error", err, "output", string(output))
	} else {
		m.logger.Info("s3 upload verified", "src", src, "dst", dst, "size", srcInfo.Size())
	}

	return nil
}

func (m *TieringManager) moveToCold(src string) {
	rel, _ := filepath.Rel(m.config.HotPath, src)

	if m.config.ColdPath == "" {
		return
	}

	dst := filepath.Join(m.config.ColdPath, rel)

	var lastErr error
	for attempt := 1; attempt <= m.config.S3Retries; attempt++ {
		if err := m.s3Upload(src, dst); err != nil {
			lastErr = err
			m.logger.Warn("s3 upload attempt failed",
				"src", src, "attempt", attempt, "max", m.config.S3Retries, "error", err)
			time.Sleep(time.Duration(attempt*attempt) * time.Second)
			continue
		}

		if err := os.Remove(src); err != nil {
			m.logger.Error("failed to remove source after cold tier upload", "src", src, "error", err)
			return
		}
		m.logger.Info("tiered to cold (S3)", "file", rel)
		return
	}

	m.logger.Error("failed to move to cold after all retries",
		"src", src, "retries", m.config.S3Retries, "error", lastErr)
}

func fileChecksum(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func isS3Path(path string) bool {
	return strings.HasPrefix(path, "s3://")
}

func s3BucketFromPath(s3path string) string {
	path := strings.TrimPrefix(s3path, "s3://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func s3KeyFromPath(s3path string) string {
	path := strings.TrimPrefix(s3path, "s3://")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
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
