package onvif

import (
	"context"
	"fmt"
	"time"
)

type ProvisioningState string

const (
	ProvisioningPending        ProvisioningState = "pending"
	ProvisioningDiscovering    ProvisioningState = "discovering"
	ProvisioningGetCapabilities ProvisioningState = "get_capabilities"
	ProvisioningGetProfiles    ProvisioningState = "get_profiles"
	ProvisioningGetStreamURI   ProvisioningState = "get_stream_uri"
	ProvisioningGetSnapshot    ProvisioningState = "get_snapshot"
	ProvisioningGetDeviceInfo  ProvisioningState = "get_device_info"
	ProvisioningDetectPTZ      ProvisioningState = "detect_ptz"
	ProvisioningDetectEvents   ProvisioningState = "detect_events"
	ProvisioningCreating       ProvisioningState = "creating"
	ProvisioningConfiguring    ProvisioningState = "configuring"
	ProvisioningVerifying      ProvisioningState = "verifying"
	ProvisioningComplete       ProvisioningState = "complete"
	ProvisioningFailed         ProvisioningState = "failed"
)

type ProvisioningReport struct {
	State       ProvisioningState
	Success     bool
	Message     string
	DeviceInfo  *DeviceInfo
	Capabilities *Capabilities
	Profiles    []Profile
	MainStream  string
	SubStream   string
	SnapshotURI string
	HasPTZ      bool
	HasEvents   bool
	CameraID    string
}

type ProvisioningConfig struct {
	BaseURL          string
	Username         string
	Password         string
	SiteID           string
	CameraName       string
	RequestTimeout   time.Duration
	EventInitialTTL  time.Duration
}

type ProvisioningStep func(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error

func RunProvisioning(ctx context.Context, cfg *ProvisioningConfig) (*ProvisioningReport, error) {
	client := NewSOAPClient(cfg.RequestTimeout, &Credentials{
		Username: cfg.Username,
		Password: cfg.Password,
	})

	report := &ProvisioningReport{
		State:   ProvisioningPending,
		Success: false,
	}

	steps := []struct {
		Name  ProvisioningState
		Step  ProvisioningStep
	}{
		{ProvisioningGetCapabilities, provisionGetCapabilities},
		{ProvisioningGetDeviceInfo, provisionGetDeviceInfo},
		{ProvisioningGetProfiles, provisionGetProfiles},
		{ProvisioningGetStreamURI, provisionGetStreamURI},
		{ProvisioningGetSnapshot, provisionGetSnapshot},
		{ProvisioningDetectPTZ, provisionDetectPTZ},
		{ProvisioningDetectEvents, provisionDetectEvents},
	}

	for _, s := range steps {
		select {
		case <-ctx.Done():
			report.State = ProvisioningFailed
			report.Message = "provisioning cancelled"
			return report, ctx.Err()
		default:
		}

		report.State = s.Name
		if err := s.Step(ctx, client, cfg, report); err != nil {
			report.State = ProvisioningFailed
			report.Message = fmt.Sprintf("step %s failed: %v", s.Name, err)
			return report, err
		}
	}

	report.State = ProvisioningComplete
	report.Success = true
	report.Message = "provisioning complete"
	return report, nil
}

func provisionGetCapabilities(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	deviceURL := BuildDeviceURL(cfg.BaseURL)
	caps, err := GetCapabilities(ctx, client, cfg.BaseURL)
	if err != nil {
		deviceURL = cfg.BaseURL
		caps, err = GetCapabilities(ctx, client, cfg.BaseURL)
		if err != nil {
			return fmt.Errorf("get capabilities: %w", err)
		}
	}
	report.Capabilities = caps
	_ = deviceURL
	return nil
}

func provisionGetDeviceInfo(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	info, err := GetDeviceInformation(ctx, client, cfg.BaseURL)
	if err != nil {
		return fmt.Errorf("get device info: %w", err)
	}
	report.DeviceInfo = info
	return nil
}

func provisionGetProfiles(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	mediaURL := BuildMediaURL(cfg.BaseURL)
	profiles, err := GetProfiles(ctx, client, mediaURL)
	if err != nil {
		return fmt.Errorf("get profiles: %w", err)
	}
	if len(profiles) == 0 {
		return fmt.Errorf("no media profiles found")
	}
	report.Profiles = profiles
	return nil
}

func provisionGetStreamURI(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	mediaURL := BuildMediaURL(cfg.BaseURL)
	if len(report.Profiles) == 0 {
		return fmt.Errorf("no profiles available for stream negotiation")
	}

	mainProfile := FindMainProfile(report.Profiles)
	if mainProfile == nil {
		return fmt.Errorf("no suitable main profile found")
	}

	uri, err := GetStreamURI(ctx, client, mediaURL, mainProfile.Token, "RTSP")
	if err != nil {
		return fmt.Errorf("get main stream URI: %w", err)
	}
	report.MainStream = uri.URI

	subProfile := FindSubProfile(report.Profiles, mainProfile.Token)
	if subProfile != nil {
		subURI, err := GetStreamURI(ctx, client, mediaURL, subProfile.Token, "RTSP")
		if err == nil {
			report.SubStream = subURI.URI
		}
	}

	return nil
}

func provisionGetSnapshot(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	mediaURL := BuildMediaURL(cfg.BaseURL)
	if len(report.Profiles) == 0 {
		return nil
	}
	mainProfile := FindMainProfile(report.Profiles)
	if mainProfile == nil {
		mainProfile = &report.Profiles[0]
	}

	uri, err := GetSnapshotURI(ctx, client, mediaURL, mainProfile.Token)
	if err != nil {
		return nil
	}
	report.SnapshotURI = uri
	return nil
}

func provisionDetectPTZ(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	if report.Capabilities == nil {
		return nil
	}
	report.HasPTZ = report.Capabilities.PTZ
	return nil
}

func provisionDetectEvents(ctx context.Context, client *SOAPClient, cfg *ProvisioningConfig, report *ProvisioningReport) error {
	if report.Capabilities == nil {
		return nil
	}
	report.HasEvents = report.Capabilities.Events
	return nil
}
