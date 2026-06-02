package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/dam-vms/dam/pkg/common"
)

type LPRResult struct {
	Plate      string    `json:"plate"`
	Region     string    `json:"region"`
	Confidence float64   `json:"confidence"`
	Box        [4]int    `json:"box"`
	Timestamp  time.Time `json:"timestamp"`
}

type LPRProcessor struct {
	enabled     bool
	openalprCmd string
	hotlist     map[string]string
	webhookURL  string
	logger      *slog.Logger
}

func NewLPRProcessor(logger *slog.Logger) *LPRProcessor {
	return &LPRProcessor{
		enabled:     os.Getenv("LPR_ENABLED") == "true",
		openalprCmd: os.Getenv("OPENALPR_CMD"),
		hotlist:     make(map[string]string),
		webhookURL:  os.Getenv("LPR_HOTLIST_WEBHOOK"),
		logger:      logger,
	}
}

func (p *LPRProcessor) Process(frame image.Image) (*LPRResult, error) {
	if !p.enabled {
		return nil, nil
	}

	tmpDir := os.TempDir()
	inputPath := filepath.Join(tmpDir, fmt.Sprintf("lpr_%d.jpg", time.Now().UnixNano()))
	f, err := os.Create(inputPath)
	if err != nil {
		return nil, err
	}
	jpeg.Encode(f, frame, &jpeg.Options{Quality: 85})
	f.Close()
	defer os.Remove(inputPath)

	cmd := exec.Command(p.openalprCmd, "-j", inputPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		p.logger.Warn("OpenALPR failed", "error", err, "stderr", stderr.String())
		return nil, nil
	}

	var result struct {
		Results []struct {
			Plate     string  `json:"plate"`
			Region    string  `json:"region"`
			Confidence float64 `json:"confidence"`
			Coordinates []struct {
				X int `json:"x"`
				Y int `json:"y"`
			} `json:"coordinates"`
		} `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("parse alpr output: %w", err)
	}

	if len(result.Results) == 0 {
		return nil, nil
	}

	best := result.Results[0]
	lpr := &LPRResult{
		Plate:      best.Plate,
		Region:     best.Region,
		Confidence: best.Confidence,
		Timestamp:  time.Now(),
	}

	if reason, ok := p.hotlist[best.Plate]; ok && p.webhookURL != "" {
		go p.fireHotlistAlert(lpr, reason)
	}

	return lpr, nil
}

func (p *LPRProcessor) fireHotlistAlert(lpr *LPRResult, reason string) {
	if err := common.ValidateWebhookURL(p.webhookURL); err != nil {
		p.logger.Error("hotlist webhook URL validation failed", "error", err)
		return
	}
	body, _ := json.Marshal(map[string]interface{}{
		"plate":      lpr.Plate,
		"reason":     reason,
		"confidence": lpr.Confidence,
		"timestamp":  lpr.Timestamp,
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(p.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		p.logger.Error("hotlist webhook failed", "error", err)
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func (p *LPRProcessor) UpdateHotlist(plates map[string]string) {
	p.hotlist = plates
}
