package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func deviceTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}

func TestGetDeviceInformation(t *testing.T) {
	resp := `<?xml version="1.0"?>
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

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	info, err := GetDeviceInformation(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetDeviceInformation failed: %v", err)
	}

	if info.Manufacturer != "TestCorp" {
		t.Errorf("expected TestCorp, got %s", info.Manufacturer)
	}
	if info.Model != "TC-100" {
		t.Errorf("expected TC-100, got %s", info.Model)
	}
	if info.FirmwareVersion != "1.2.3" {
		t.Errorf("expected 1.2.3, got %s", info.FirmwareVersion)
	}
	if info.SerialNumber != "SN12345" {
		t.Errorf("expected SN12345, got %s", info.SerialNumber)
	}
	if info.HardwareID != "HW-V2" {
		t.Errorf("expected HW-V2, got %s", info.HardwareID)
	}
}

func TestGetCapabilitiesAll(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetCapabilitiesResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Capabilities>
        <Analytics XAddr="http://cam/analytics"/>
        <Device XAddr="http://cam/device"/>
        <Events XAddr="http://cam/events"/>
        <Imaging XAddr="http://cam/imaging"/>
        <Media XAddr="http://cam/media" SnapshotUri="true"/>
        <PTZ XAddr="http://cam/ptz"/>
        <Recording XAddr="http://cam/recording"/>
        <Replay XAddr="http://cam/replay"/>
      </Capabilities>
    </GetCapabilitiesResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	caps, err := GetCapabilities(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if !caps.Analytics { t.Error("Analytics should be true") }
	if !caps.Device { t.Error("Device should be true") }
	if !caps.Events { t.Error("Events should be true") }
	if !caps.Imaging { t.Error("Imaging should be true") }
	if !caps.Media { t.Error("Media should be true") }
	if !caps.PTZ { t.Error("PTZ should be true") }
	if !caps.Recording { t.Error("Recording should be true") }
	if !caps.Replay { t.Error("Replay should be true") }
}

func TestGetCapabilitiesPartial(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetCapabilitiesResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Capabilities>
        <Media XAddr="http://cam/media"/>
        <PTZ XAddr="http://cam/ptz"/>
      </Capabilities>
    </GetCapabilitiesResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	caps, err := GetCapabilities(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetCapabilities failed: %v", err)
	}

	if caps.Analytics { t.Error("Analytics should be false") }
	if caps.Imaging { t.Error("Imaging should be false") }
	if caps.Events { t.Error("Events should be false") }
	if !caps.Media { t.Error("Media should be true") }
	if !caps.PTZ { t.Error("PTZ should be true") }
}

func TestGetServices(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetServicesResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Service>
        <Namespace>http://www.onvif.org/ver10/media/wsdl</Namespace>
        <XAddr>http://cam/onvif/media_service</XAddr>
        <Version><Major>20</Major><Minor>12</Minor></Version>
      </Service>
      <Service>
        <Namespace>http://www.onvif.org/ver20/ptz/wsdl</Namespace>
        <XAddr>http://cam/onvif/ptz_service</XAddr>
        <Version><Major>20</Major><Minor>12</Minor></Version>
      </Service>
    </GetServicesResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	services, err := GetServices(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetServices failed: %v", err)
	}

	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
	}

	if !strings.Contains(services[0].Namespace, "media") {
		t.Error("first service should be media")
	}
}

func TestGetHostname(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetHostnameResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <Name>Camera-01</Name>
    </GetHostnameResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	hostname, err := GetHostname(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetHostname failed: %v", err)
	}

	if hostname != "Camera-01" {
		t.Errorf("expected Camera-01, got %s", hostname)
	}
}

func TestGetSystemDateAndTime(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetSystemDateAndTimeResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <UTCDateTime>2024-01-15T10:30:00Z</UTCDateTime>
      <TimeZone><TZ>UTC</TZ></TimeZone>
    </GetSystemDateAndTimeResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	result, err := GetSystemDateAndTime(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetSystemDateAndTime failed: %v", err)
	}

	if result["utc_date_time"] != "2024-01-15T10:30:00Z" {
		t.Errorf("unexpected UTC time: %v", result["utc_date_time"])
	}
}

func TestGetNetworkInterfaces(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetNetworkInterfacesResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <NetworkInterfaces>
        <Name>eth0</Name>
        <HwAddress>00:11:22:33:44:55</HwAddress>
        <MTU>1500</MTU>
        <IPv4><Enabled>true</Enabled><Address>192.168.1.100</Address><PrefixLength>24</PrefixLength></IPv4>
      </NetworkInterfaces>
    </GetNetworkInterfacesResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	ifaces, err := GetNetworkInterfaces(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetNetworkInterfaces failed: %v", err)
	}

	if len(ifaces) != 1 {
		t.Fatalf("expected 1 interface, got %d", len(ifaces))
	}

	if ifaces[0]["name"] != "eth0" {
		t.Errorf("expected eth0, got %v", ifaces[0]["name"])
	}

	if ifaces[0]["ipv4_address"] != "192.168.1.100" {
		t.Errorf("expected 192.168.1.100, got %v", ifaces[0]["ipv4_address"])
	}
}

func TestGetDNS(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetDNSResponse xmlns="http://www.onvif.org/ver10/device/wsdl">
      <FromDHCP>true</FromDHCP>
      <DNSManual>8.8.8.8</DNSManual>
      <DNSManual>8.8.4.4</DNSManual>
    </GetDNSResponse>
  </s:Body>
</s:Envelope>`

	server := deviceTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	result, err := GetDNS(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetDNS failed: %v", err)
	}

	if result["from_dhcp"] != "true" {
		t.Errorf("expected true, got %v", result["from_dhcp"])
	}
}
