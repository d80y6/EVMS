package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSOAPClient(t *testing.T) {
	client := NewSOAPClient(10*time.Second, &Credentials{Username: "admin", Password: "pass"})

	if client == nil {
		t.Fatal("client should not be nil")
	}

	if client.httpClient.Timeout != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", client.httpClient.Timeout)
	}
}

func TestSOAPClientDoWithAuth(t *testing.T) {
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedBody = r.Method
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <TestResponse><Token>test123</Token></TestResponse>
  </s:Body>
</s:Envelope>`))
	}))
	defer server.Close()

	_ = receivedBody
	client := NewSOAPClient(5*time.Second, &Credentials{Username: "admin", Password: "pass"})
	data, err := client.Do(context.Background(), server.URL, "", "<Test/>")
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	if !strings.Contains(string(data), "test123") {
		t.Error("response should contain test123 token")
	}
}

func TestSOAPClientDoWithoutAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body><Response>ok</Response></s:Body>
</s:Envelope>`))
	}))
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	data, err := client.Do(context.Background(), server.URL, "", "<Test/>")
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}

	if !strings.Contains(string(data), "ok") {
		t.Error("response should contain ok")
	}
}

func TestExtractFault(t *testing.T) {
	soapFault := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <s:Fault>
      <s:Code><s:Value>s:Sender</s:Value></s:Code>
      <s:Reason><s:Text>Invalid token</s:Text></s:Reason>
    </s:Fault>
  </s:Body>
</s:Envelope>`

	fault := ExtractFault([]byte(soapFault))
	if fault == nil {
		t.Fatal("fault should not be nil")
	}

	if fault.Code != "s:Sender" {
		t.Errorf("expected s:Sender, got %s", fault.Code)
	}

	if fault.Reason != "Invalid token" {
		t.Errorf("expected 'Invalid token', got '%s'", fault.Reason)
	}
}

func TestCheckSOAPFaultNoFault(t *testing.T) {
	data := []byte(`<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body><Response>ok</Response></s:Body>
</s:Envelope>`)

	err := CheckSOAPFault(data)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestBuildMediaURL(t *testing.T) {
	url := BuildMediaURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/media_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildMediaURLWithPath(t *testing.T) {
	url := BuildMediaURL("http://192.168.1.100/onvif/device_service")
	if url != "http://192.168.1.100/onvif/device_service/onvif/media_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildPTZURL(t *testing.T) {
	url := BuildPTZURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/ptz_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildDeviceURL(t *testing.T) {
	url := BuildDeviceURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/device_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildEventURL(t *testing.T) {
	url := BuildEventURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/event_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildImagingURL(t *testing.T) {
	url := BuildImagingURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/imaging_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildRecordingURL(t *testing.T) {
	url := BuildRecordingURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/recording_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestBuildDeviceIOURL(t *testing.T) {
	url := BuildDeviceIOURL("http://192.168.1.100")
	if url != "http://192.168.1.100/onvif/deviceio_service" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestExtractXMLString(t *testing.T) {
	data := []byte(`<root><Token>abc123</Token></root>`)
	val, err := ExtractXMLString(data, "Token")
	if err != nil {
		t.Fatalf("ExtractXMLString failed: %v", err)
	}
	if val != "abc123" {
		t.Errorf("expected abc123, got %s", val)
	}
}

func TestExtractXMLStringNotFound(t *testing.T) {
	data := []byte(`<root><Other>value</Other></root>`)
	_, err := ExtractXMLString(data, "Token")
	if err == nil {
		t.Error("expected error for missing tag")
	}
}

func TestSOAPBuilder(t *testing.T) {
	builder := NewSOAPBuilder()
	body := "<Test>data</Test>"
	result := builder.Build(body)

	if !strings.Contains(result, "<soap:Envelope") {
		t.Error("should contain soap envelope")
	}

	if !strings.Contains(result, "<Test>data</Test>") {
		t.Error("should contain body content")
	}

	if !strings.Contains(result, "<soap:Body>") {
		t.Error("should contain soap body element")
	}
}

func TestSOAPBuilderWithNamespaces(t *testing.T) {
	builder := NewSOAPBuilder()
	builder.WithNamespace("trt", "http://www.onvif.org/ver10/media/wsdl")

	body := "<trt:GetProfiles/>"
	result := builder.Build(body)

	if !strings.Contains(result, `xmlns:trt="http://www.onvif.org/ver10/media/wsdl"`) {
		t.Error("should contain namespace declaration for trt")
	}
}

func TestSOAPBuilderWithHeader(t *testing.T) {
	builder := NewSOAPBuilder()
	header := "<wsse:Security>token</wsse:Security>"
	body := "<Test/>"
	result := builder.WithHeader(header).Build(body)

	if !strings.Contains(result, "<wsse:Security>token</wsse:Security>") {
		t.Error("should contain security header")
	}
}

func TestNewSOAPBuilder(t *testing.T) {
	builder := NewSOAPBuilder()
	if builder == nil {
		t.Fatal("builder should not be nil")
	}
	if _, ok := builder.namespaces["soap"]; !ok {
		t.Error("should have soap namespace by default")
	}
}
