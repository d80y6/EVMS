package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreatePullPointSubscription(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <CreatePullPointSubscriptionResponse xmlns="http://docs.oasis-open.org/wsn/b-2">
      <SubscriptionReference>
        <Address>http://cam/pullpoint/abc123</Address>
      </SubscriptionReference>
      <CurrentTime>2024-01-15T10:00:00Z</CurrentTime>
      <TerminationTime>2024-01-15T11:00:00Z</TerminationTime>
    </CreatePullPointSubscriptionResponse>
  </s:Body>
</s:Envelope>`

	server := eventTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	sub, err := CreatePullPointSubscription(context.Background(), client, server.URL, 3600*time.Second)
	if err != nil {
		t.Fatalf("CreatePullPointSubscription failed: %v", err)
	}

	if sub.Address != "http://cam/pullpoint/abc123" {
		t.Errorf("expected http://cam/pullpoint/abc123, got %s", sub.Address)
	}
}

func TestPullMessages(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <PullMessagesResponse xmlns="http://docs.oasis-open.org/wsn/b-2">
      <NotificationMessage>
        <Topic>MotionAlarm</Topic>
        <Message><Data>motion detected</Data></Message>
      </NotificationMessage>
      <NotificationMessage>
        <Topic>Tampering</Topic>
        <Message><Data>tamper detected</Data></Message>
      </NotificationMessage>
    </PullMessagesResponse>
  </s:Body>
</s:Envelope>`

	server := eventTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	events, err := PullMessages(context.Background(), client, server.URL, 10, 5*time.Second)
	if err != nil {
		t.Fatalf("PullMessages failed: %v", err)
	}

	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}

	if events[0].Topic != "MotionAlarm" {
		t.Errorf("expected MotionAlarm, got %s", events[0].Topic)
	}
}

func TestPullMessagesNoEvents(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <PullMessagesResponse xmlns="http://docs.oasis-open.org/wsn/b-2"/>
  </s:Body>
</s:Envelope>`

	server := eventTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	events, err := PullMessages(context.Background(), client, server.URL, 10, 5*time.Second)
	if err != nil {
		t.Fatal("no events should not be an error")
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestRenewPullPointSubscription(t *testing.T) {
	server := eventTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><RenewResponse xmlns="http://docs.oasis-open.org/wsn/b-2"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := RenewPullPointSubscription(context.Background(), client, server.URL, 3600*time.Second)
	if err != nil {
		t.Fatalf("RenewPullPointSubscription failed: %v", err)
	}
}

func TestUnsubscribePullPoint(t *testing.T) {
	server := eventTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><UnsubscribeResponse xmlns="http://docs.oasis-open.org/wsn/b-2"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := UnsubscribePullPoint(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("UnsubscribePullPoint failed: %v", err)
	}
}

func TestClassifyEventTopic(t *testing.T) {
	tests := []struct {
		topic string
		want  string
	}{
		{"MotionAlarm", "motion"},
		{"CellMotionDetector/Motion", "cell_motion"},
		{"tns:RuleEngine/CellMotionDetector/Motion", "cell_motion"},
		{"tns:RuleEngine/Tampering", "tamper"},
		{"Tamper", "tamper"},
		{"Alarm", "alarm"},
		{"Relay", "relay"},
		{"DigitalInput", "digital_input"},
		{"LineCrossing", "line_crossing"},
		{"PeopleCounting", "people_count"},
		{"FaceDetection", "face_detection"},
		{"Parking", "parking"},
		{"Loitering", "loitering"},
		{"UnknownTopic", ""},
	}

	for _, tt := range tests {
		got := ClassifyEventTopic(tt.topic)
		if got != tt.want {
			t.Errorf("ClassifyEventTopic(%q) = %q, want %q", tt.topic, got, tt.want)
		}
	}
}

func TestGetEventProperties(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetEventPropertiesResponse xmlns="http://docs.oasis-open.org/wsn/b-2">
      <Topic>Motion</Topic>
      <Topic>Tamper</Topic>
      <Topic>Alarm</Topic>
    </GetEventPropertiesResponse>
  </s:Body>
</s:Envelope>`

	server := eventTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	topics, err := GetEventProperties(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetEventProperties failed: %v", err)
	}

	if len(topics) != 3 {
		t.Fatalf("expected 3 topics, got %d", len(topics))
	}

	if topics[0] != "Motion" {
		t.Errorf("expected Motion, got %s", topics[0])
	}
}

func eventTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}
