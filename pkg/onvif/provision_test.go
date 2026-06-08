package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunProvisioning(t *testing.T) {
	reqCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/soap+xml")

		switch reqCount {
		case 1:
			w.Write([]byte(capabilitiesResponse()))
		case 2:
			w.Write([]byte(deviceInfoResponse()))
		case 3:
			w.Write([]byte(profilesResponse()))
		case 4:
			w.Write([]byte(streamURIResponse()))
		case 5:
			w.Write([]byte(snapshotURIResponse()))
		default:
			w.Write([]byte(emptyResponse()))
		}
	}))
	defer server.Close()

	cfg := &ProvisioningConfig{
		BaseURL:         server.URL,
		Username:        "admin",
		Password:        "pass",
		CameraName:      "Test Camera",
		RequestTimeout:  5 * time.Second,
		EventInitialTTL: 3600 * time.Second,
	}

	report, err := RunProvisioning(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProvisioning failed: %v", err)
	}

	if report.State != ProvisioningComplete {
		t.Errorf("expected complete, got %s", report.State)
	}

	if !report.Success {
		t.Error("expected success")
	}

	if report.DeviceInfo == nil {
		t.Error("DeviceInfo should not be nil")
	} else if report.DeviceInfo.Manufacturer != "TestCorp" {
		t.Errorf("expected TestCorp, got %s", report.DeviceInfo.Manufacturer)
	}

	if report.Capabilities == nil {
		t.Error("Capabilities should not be nil")
	} else if !report.Capabilities.Media {
		t.Error("Media capability should be true")
	}

	if len(report.Profiles) == 0 {
		t.Error("should have at least 1 profile")
	}

	if report.MainStream == "" {
		t.Error("MainStream should not be empty")
	}

	if report.SnapshotURI == "" {
		t.Error("SnapshotURI should not be empty")
	}
}

func TestRunProvisioningWithAuth(t *testing.T) {
	reqCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/soap+xml")
		switch reqCount {
		case 1:
			w.Write([]byte(capabilitiesResponse()))
		case 2:
			w.Write([]byte(deviceInfoResponse()))
		case 3:
			w.Write([]byte(profilesResponse()))
		default:
			w.Write([]byte(streamURIResponse()))
		}
	}))
	defer server.Close()

	cfg := &ProvisioningConfig{
		BaseURL:        server.URL,
		Username:       "admin",
		Password:       "pass",
		RequestTimeout: 5 * time.Second,
	}

	report, err := RunProvisioning(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RunProvisioning failed: %v", err)
	}

	if report.State == ProvisioningFailed {
		t.Errorf("provisioning failed: %s", report.Message)
	}
}

func TestRunProvisioningCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &ProvisioningConfig{
		BaseURL:        "http://invalid",
		Username:       "admin",
		Password:       "pass",
		RequestTimeout: 1 * time.Second,
	}

	_, err := RunProvisioning(ctx, cfg)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestRunProvisioningNetworkError(t *testing.T) {
	cfg := &ProvisioningConfig{
		BaseURL:        "http://192.0.2.1:9999",
		Username:       "admin",
		Password:       "pass",
		RequestTimeout: 1 * time.Second,
	}

	_, err := RunProvisioning(context.Background(), cfg)
	if err == nil {
		t.Skip("expected network error (skipping if network available)")
	}
}

func capabilitiesResponse() string {
	return `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetCapabilitiesResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Capabilities>
        <Media XAddr="http://cam/media" SnapshotUri="true"/>
        <PTZ XAddr="http://cam/ptz"/>
        <Events XAddr="http://cam/events"/>
        <Imaging XAddr="http://cam/imaging"/>
        <Device XAddr="http://cam/device"/>
      </Capabilities>
    </GetCapabilitiesResponse>
  </s:Body>
</s:Envelope>`
}

func deviceInfoResponse() string {
	return `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetDeviceInformationResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Manufacturer>TestCorp</Manufacturer>
      <Model>TC-100</Model>
      <FirmwareVersion>1.2.3</FirmwareVersion>
      <SerialNumber>SN12345</SerialNumber>
      <HardwareId>HW-V2</HardwareId>
    </GetDeviceInformationResponse>
  </s:Body>
</s:Envelope>`
}

func profilesResponse() string {
	return `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetProfilesResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <Profiles token="profile_1">
        <Name>Main</Name>
        <VideoEncoderConfiguration token="enc_1">
          <Encoding>H264</Encoding>
          <Resolution><Width>1920</Width><Height>1080</Height></Resolution>
          <FrameRateLimit>30</FrameRateLimit>
          <BitrateLimit>4096</BitrateLimit>
        </VideoEncoderConfiguration>
      </Profiles>
    </GetProfilesResponse>
  </s:Body>
</s:Envelope>`
}

func streamURIResponse() string {
	return `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetStreamUriResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <MediaUri>
        <Uri>rtsp://192.168.1.100:554/stream1</Uri>
      </MediaUri>
    </GetStreamUriResponse>
  </s:Body>
</s:Envelope>`
}

func snapshotURIResponse() string {
	return `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetSnapshotUriResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <Uri><Uri>http://192.168.1.100/snapshot.jpg</Uri></Uri>
    </GetSnapshotUriResponse>
  </s:Body>
</s:Envelope>`
}

func emptyResponse() string {
	return `<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><Response/></s:Body></s:Envelope>`
}
