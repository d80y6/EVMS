package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetAnalyticsModules(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetAnalyticsModulesResponse xmlns="http://www.onvif.org/ver20/analytics/wsdl">
      <AnalyticsModule token="am_1">
        <Name>Motion Detector</Name>
        <Type>MotionDetection</Type>
      </AnalyticsModule>
      <AnalyticsModule token="am_2">
        <Name>Line Crossing</Name>
        <Type>LineCrossing</Type>
      </AnalyticsModule>
    </GetAnalyticsModulesResponse>
  </s:Body>
</s:Envelope>`

	server := analyticsTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	modules, err := GetAnalyticsModules(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetAnalyticsModules failed: %v", err)
	}

	if len(modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(modules))
	}

	if modules[0].Name != "Motion Detector" {
		t.Errorf("expected Motion Detector, got %s", modules[0].Name)
	}
}

func TestGetSupportedAnalyticsRules(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetSupportedRulesResponse xmlns="http://www.onvif.org/ver20/analytics/wsdl">
      <Rule token="rule_motion">
        <Name>MotionDetection</Name>
      </Rule>
      <Rule token="rule_linecross">
        <Name>LineCrossing</Name>
      </Rule>
    </GetSupportedRulesResponse>
  </s:Body>
</s:Envelope>`

	server := analyticsTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	rules, err := GetSupportedAnalyticsRules(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetSupportedAnalyticsRules failed: %v", err)
	}

	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
}

func TestParseAnalyticsMetadata(t *testing.T) {
	xmlData := `<?xml version="1.0"?>
<MetadataStream>
  <VideoAnalytics>
    <Object ObjectId="1">
      <Appearance Class="Person" Confidence="0.95">
        <Shape>
          <BoundingBox left="0.1" top="0.2" right="0.3" bottom="0.5"/>
          <Center x="0.2" y="0.35"/>
        </Shape>
      </Appearance>
    </Object>
    <Object ObjectId="2">
      <Appearance Class="Vehicle" Confidence="0.85">
        <Shape>
          <BoundingBox left="0.4" top="0.3" right="0.6" bottom="0.7"/>
        </Shape>
      </Appearance>
    </Object>
  </VideoAnalytics>
</MetadataStream>`

	objects, err := ParseAnalyticsMetadata([]byte(xmlData))
	if err != nil {
		t.Fatalf("ParseAnalyticsMetadata failed: %v", err)
	}

	if len(objects) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objects))
	}

	if objects[0].Type != "Person" {
		t.Errorf("expected Person, got %s", objects[0].Type)
	}

	if objects[0].BoundingBox == nil {
		t.Fatal("BoundingBox should not be nil")
	}

	if objects[0].BoundingBox.Left != 0.1 {
		t.Errorf("expected 0.1, got %f", objects[0].BoundingBox.Left)
	}
}

func TestParseAnalyticsMetadataEmpty(t *testing.T) {
	objects, err := ParseAnalyticsMetadata([]byte(`<?xml version="1.0"?><MetadataStream/>`))
	if err != nil {
		t.Fatalf("ParseAnalyticsMetadata failed: %v", err)
	}

	if len(objects) != 0 {
		t.Errorf("expected 0 objects, got %d", len(objects))
	}
}

func TestCreateAnalyticsRule(t *testing.T) {
	server := analyticsTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><CreateRuleResponse xmlns="http://www.onvif.org/ver20/analytics/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := CreateAnalyticsRule(context.Background(), client, server.URL, "rule_motion", "MotionDetection",
		map[string]string{"sensitivity": "0.5"})
	if err != nil {
		t.Fatalf("CreateAnalyticsRule failed: %v", err)
	}
}

func TestDeleteAnalyticsRule(t *testing.T) {
	server := analyticsTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><DeleteRuleResponse xmlns="http://www.onvif.org/ver20/analytics/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := DeleteAnalyticsRule(context.Background(), client, server.URL, "rule_motion")
	if err != nil {
		t.Fatalf("DeleteAnalyticsRule failed: %v", err)
	}
}

func analyticsTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}
