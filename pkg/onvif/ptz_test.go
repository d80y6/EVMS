package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetPresets(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetPresetsResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <Preset token="p1"><Name>Entrance</Name></Preset>
      <Preset token="p2"><Name>Parking Lot</Name></Preset>
      <Preset token="p3"><Name>Lobby</Name></Preset>
    </GetPresetsResponse>
  </s:Body>
</s:Envelope>`

	server := ptzTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	presets, err := GetPresets(context.Background(), client, server.URL, "profile_1")
	if err != nil {
		t.Fatalf("GetPresets failed: %v", err)
	}

	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d", len(presets))
	}

	if presets[0].Token != "p1" {
		t.Errorf("expected p1, got %s", presets[0].Token)
	}
	if presets[0].Name != "Entrance" {
		t.Errorf("expected Entrance, got %s", presets[0].Name)
	}
	if presets[1].Name != "Parking Lot" {
		t.Errorf("expected Parking Lot, got %s", presets[1].Name)
	}
}

func TestGetPresetsEmpty(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetPresetsResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
    </GetPresetsResponse>
  </s:Body>
</s:Envelope>`

	server := ptzTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	presets, err := GetPresets(context.Background(), client, server.URL, "profile_1")
	if err != nil {
		t.Fatalf("GetPresets failed: %v", err)
	}

	if len(presets) != 0 {
		t.Errorf("expected 0 presets, got %d", len(presets))
	}
}

func TestSetPreset(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <SetPresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <PresetToken>p_new_1</PresetToken>
    </SetPresetResponse>
  </s:Body>
</s:Envelope>`

	server := ptzTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	token, err := SetPreset(context.Background(), client, server.URL, "profile_1", "Main Entrance")
	if err != nil {
		t.Fatalf("SetPreset failed: %v", err)
	}

	if token != "p_new_1" {
		t.Errorf("expected p_new_1, got %s", token)
	}
}

func TestRemovePreset(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><RemovePresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := RemovePreset(context.Background(), client, server.URL, "profile_1", "p1")
	if err != nil {
		t.Fatalf("RemovePreset failed: %v", err)
	}
}

func TestContinuousMove(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := ContinuousMove(context.Background(), client, server.URL, "profile_1",
		&Vector2D{X: 0.5, Y: 0.5}, nil)
	if err != nil {
		t.Fatalf("ContinuousMove failed: %v", err)
	}
}

func TestStop(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><StopResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := Stop(context.Background(), client, server.URL, "profile_1", true, true)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestAbsoluteMove(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><AbsoluteMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := AbsoluteMove(context.Background(), client, server.URL, "profile_1",
		&PTZPosition{
			PanTilt: &Vector2D{X: 0.5, Y: 0.3, Space: "http://www.onvif.org/ver10/tptz/PanTiltSpaces/PositionSpace"},
		}, nil)
	if err != nil {
		t.Fatalf("AbsoluteMove failed: %v", err)
	}
}

func TestRelativeMove(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><RelativeMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := RelativeMove(context.Background(), client, server.URL, "profile_1",
		&Vector2D{X: 0.1, Y: 0.1}, nil, nil)
	if err != nil {
		t.Fatalf("RelativeMove failed: %v", err)
	}
}

func TestGotoHomePosition(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GotoHomePositionResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := GotoHomePosition(context.Background(), client, server.URL, "profile_1", nil)
	if err != nil {
		t.Fatalf("GotoHomePosition failed: %v", err)
	}
}

func TestSetHomePosition(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><SetHomePositionResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := SetHomePosition(context.Background(), client, server.URL, "profile_1")
	if err != nil {
		t.Fatalf("SetHomePosition failed: %v", err)
	}
}

func TestGetPTZStatus(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetStatusResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <PTZStatus>
        <Position>
          <PanTilt x="0.5" y="0.3" space="http://www.onvif.org/ver10/tptz/PanTiltSpaces/PositionSpace"/>
          <Zoom x="0.1" space="http://www.onvif.org/ver10/tptz/ZoomSpaces/PositionSpace"/>
        </Position>
        <MoveStatus><PanTilt>IDLE</PanTilt><Zoom>IDLE</Zoom></MoveStatus>
        <UtcTime>2024-01-15T10:30:00Z</UtcTime>
      </PTZStatus>
    </GetStatusResponse>
  </s:Body>
</s:Envelope>`

	server := ptzTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	status, err := GetPTZStatus(context.Background(), client, server.URL, "profile_1")
	if err != nil {
		t.Fatalf("GetPTZStatus failed: %v", err)
	}

	if status.MoveStatus == nil {
		t.Fatal("MoveStatus should not be nil")
	}
	if status.MoveStatus.PanTilt != "IDLE" {
		t.Errorf("expected IDLE, got %s", status.MoveStatus.PanTilt)
	}
}

func TestGetPresetsEmptyResponse(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetPresetsResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/>
  </s:Body>
</s:Envelope>`

	server := ptzTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	presets, err := GetPresets(context.Background(), client, server.URL, "profile_1")
	if err != nil {
		t.Fatalf("GetPresets failed on empty response: %v", err)
	}

	if presets == nil {
		t.Error("presets should be empty slice, not nil")
	}
}

func TestGotoPreset(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GotoPresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := GotoPreset(context.Background(), client, server.URL, "profile_1", "p1", nil)
	if err != nil {
		t.Fatalf("GotoPreset failed: %v", err)
	}
}

func TestGotoPresetWithSpeed(t *testing.T) {
	server := ptzTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><GotoPresetResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := GotoPreset(context.Background(), client, server.URL, "profile_1", "p1",
		&PTZSpeed{
			PanTilt: &Vector2D{X: 0.5, Y: 0.5},
		})
	if err != nil {
		t.Fatalf("GotoPreset with speed failed: %v", err)
	}
}

func TestGetNode(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetNodeResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl">
      <PTZNode token="ptz_node_1">
        <Name>PTZ Node 1</Name>
        <SupportedPTZSpaces>
          <AbsolutePanTiltPositionSpace><URI>http://www.onvif.org/ver10/tptz/PanTiltSpaces/PositionSpace</URI></AbsolutePanTiltPositionSpace>
          <AbsoluteZoomPositionSpace><URI>http://www.onvif.org/ver10/tptz/ZoomSpaces/PositionSpace</URI></AbsoluteZoomPositionSpace>
          <ContinuousPanTiltVelocitySpace><URI>http://www.onvif.org/ver10/tptz/PanTiltSpaces/VelocitySpace</URI></ContinuousPanTiltVelocitySpace>
          <ContinuousZoomVelocitySpace><URI>http://www.onvif.org/ver10/tptz/ZoomSpaces/VelocitySpace</URI></ContinuousZoomVelocitySpace>
        </SupportedPTZSpaces>
      </PTZNode>
    </GetNodeResponse>
  </s:Body>
</s:Envelope>`

	server := ptzTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	result, err := GetNode(context.Background(), client, server.URL, "ptz_node_1")
	if err != nil {
		t.Fatalf("GetNode failed: %v", err)
	}

	if result["token"] != "ptz_node_1" {
		t.Errorf("expected ptz_node_1, got %v", result["token"])
	}
	if result["has_absolute_pan_tilt"] != true {
		t.Error("expected absolute pan/tilt support")
	}
	if result["has_continuous_pan_tilt"] != true {
		t.Error("expected continuous pan/tilt support")
	}
}

func TestContinuousMoveWithZoom(t *testing.T) {
	var requestBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		requestBody = string(buf[:n])
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><ContinuousMoveResponse xmlns="http://www.onvif.org/ver20/ptz/wsdl"/></s:Body></s:Envelope>`))
	}))
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := ContinuousMove(context.Background(), client, server.URL, "profile_1",
		&Vector2D{X: 0.0, Y: 0.5},
		&Vector1D{X: 0.3})
	if err != nil {
		t.Fatalf("ContinuousMove with zoom failed: %v", err)
	}

	if !strings.Contains(requestBody, "profile_1") {
		t.Error("request should contain profile token")
	}
	if !strings.Contains(requestBody, "0.5") {
		t.Error("request should contain y velocity")
	}
	if !strings.Contains(requestBody, "0.3") {
		t.Error("request should contain zoom velocity")
	}
}

func ptzTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}
