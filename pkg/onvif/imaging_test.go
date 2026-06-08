package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetImagingSettings(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetImagingSettingsResponse xmlns="http://www.onvif.org/ver20/imaging/wsdl">
      <ImagingSettings>
        <Brightness>50</Brightness>
        <ColorSaturation>60</ColorSaturation>
        <Contrast>50</Contrast>
        <Sharpness>50</Sharpness>
        <Exposure>
          <Mode>MANUAL</Mode>
          <ExposureTime>8.33</ExposureTime>
          <Gain>20</Gain>
          <Iris>50</Iris>
        </Exposure>
        <WhiteBalance>
          <Mode>AUTO</Mode>
          <CrGain>100</CrGain>
          <CbGain>100</CbGain>
        </WhiteBalance>
        <WideDynamicRange>
          <Mode>OFF</Mode>
          <Level>0</Level>
        </WideDynamicRange>
        <BacklightCompensation>
          <Mode>OFF</Mode>
          <Level>0</Level>
        </BacklightCompensation>
        <IrCutFilter>ON</IrCutFilter>
      </ImagingSettings>
    </GetImagingSettingsResponse>
  </s:Body>
</s:Envelope>`

	server := imagingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	settings, err := GetImagingSettings(context.Background(), client, server.URL, "vs_1")
	if err != nil {
		t.Fatalf("GetImagingSettings failed: %v", err)
	}

	if settings.Brightness == nil || *settings.Brightness != 50 {
		t.Errorf("expected Brightness 50, got %v", settings.Brightness)
	}
	if settings.Contrast == nil || *settings.Contrast != 50 {
		t.Errorf("expected Contrast 50, got %v", settings.Contrast)
	}
	if settings.Exposure == nil {
		t.Fatal("Exposure should not be nil")
	}
	if settings.Exposure.Mode != "MANUAL" {
		t.Errorf("expected MANUAL exposure mode, got %s", settings.Exposure.Mode)
	}
	if settings.WhiteBalance == nil || settings.WhiteBalance.Mode != "AUTO" {
		t.Errorf("expected AUTO white balance, got %v", settings.WhiteBalance)
	}
	if settings.WideDynamicRange == nil || settings.WideDynamicRange.Mode != "OFF" {
		t.Errorf("expected OFF WDR, got %v", settings.WideDynamicRange)
	}
	if settings.BacklightCompensation == nil || settings.BacklightCompensation.Mode != "OFF" {
		t.Errorf("expected OFF BLC, got %v", settings.BacklightCompensation)
	}
	if settings.IrCutFilter != "ON" {
		t.Errorf("expected ON IrCutFilter, got %s", settings.IrCutFilter)
	}
}

func TestGetImagingSettingsMinimal(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetImagingSettingsResponse xmlns="http://www.onvif.org/ver20/imaging/wsdl">
      <ImagingSettings>
        <Brightness>50</Brightness>
        <Contrast>50</Contrast>
      </ImagingSettings>
    </GetImagingSettingsResponse>
  </s:Body>
</s:Envelope>`

	server := imagingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	settings, err := GetImagingSettings(context.Background(), client, server.URL, "vs_1")
	if err != nil {
		t.Fatalf("GetImagingSettings failed: %v", err)
	}

	if settings.Exposure != nil {
		t.Error("Exposure should be nil for minimal response")
	}
	if settings.WhiteBalance != nil {
		t.Error("WhiteBalance should be nil for minimal response")
	}
}

func TestSetImagingSettings(t *testing.T) {
	server := imagingTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><SetImagingSettingsResponse xmlns="http://www.onvif.org/ver20/imaging/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	brightness := 75.0
	contrast := 60.0
	settings := &ImagingSettings{
		Brightness: &brightness,
		Contrast:   &contrast,
	}

	err := SetImagingSettings(context.Background(), client, server.URL, "vs_1", settings)
	if err != nil {
		t.Fatalf("SetImagingSettings failed: %v", err)
	}
}

func TestMoveFocus(t *testing.T) {
	server := imagingTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><MoveResponse xmlns="http://www.onvif.org/ver20/imaging/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := MoveFocus(context.Background(), client, server.URL, "vs_1", 0.5)
	if err != nil {
		t.Fatalf("MoveFocus failed: %v", err)
	}
}

func TestStopFocus(t *testing.T) {
	server := imagingTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><StopResponse xmlns="http://www.onvif.org/ver20/imaging/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := StopFocus(context.Background(), client, server.URL, "vs_1")
	if err != nil {
		t.Fatalf("StopFocus failed: %v", err)
	}
}

func TestGetImagingStatus(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetStatusResponse xmlns="http://www.onvif.org/ver20/imaging/wsdl">
      <ImagingStatus>
        <FocusState>FOCUSED</FocusState>
      </ImagingStatus>
    </GetStatusResponse>
  </s:Body>
</s:Envelope>`

	server := imagingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	status, err := GetImagingStatus(context.Background(), client, server.URL, "vs_1")
	if err != nil {
		t.Fatalf("GetImagingStatus failed: %v", err)
	}

	if status.FocusState != "FOCUSED" {
		t.Errorf("expected FOCUSED, got %s", status.FocusState)
	}
}

func imagingTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}
