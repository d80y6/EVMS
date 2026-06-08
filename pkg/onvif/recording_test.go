package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetRecordings(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetRecordingsResponse xmlns="http://www.onvif.org/ver10/recording/wsdl">
      <Recording token="rec_1">
        <Source token="vs_1"/>
        <Content>Video</Content>
      </Recording>
      <Recording token="rec_2">
        <Source token="vs_1"/>
        <Content>Audio</Content>
      </Recording>
    </GetRecordingsResponse>
  </s:Body>
</s:Envelope>`

	server := recordingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	recordings, err := GetRecordings(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetRecordings failed: %v", err)
	}

	if len(recordings) != 2 {
		t.Fatalf("expected 2 recordings, got %d", len(recordings))
	}

	if recordings[0].Token != "rec_1" {
		t.Errorf("expected rec_1, got %s", recordings[0].Token)
	}
	if recordings[0].Configuration.Content != "Video" {
		t.Errorf("expected Video, got %s", recordings[0].Configuration.Content)
	}
}

func TestCreateRecording(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <CreateRecordingResponse xmlns="http://www.onvif.org/ver10/recording/wsdl">
      <RecordingToken>rec_new_1</RecordingToken>
    </CreateRecordingResponse>
  </s:Body>
</s:Envelope>`

	server := recordingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	token, err := CreateRecording(context.Background(), client, server.URL, &RecordingConfiguration{
		Source:  "vs_1",
		Content: "Video",
	})
	if err != nil {
		t.Fatalf("CreateRecording failed: %v", err)
	}

	if token != "rec_new_1" {
		t.Errorf("expected rec_new_1, got %s", token)
	}
}

func TestDeleteRecording(t *testing.T) {
	server := recordingTestServer(`<?xml version="1.0"?><s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"><s:Body><DeleteRecordingResponse xmlns="http://www.onvif.org/ver10/recording/wsdl"/></s:Body></s:Envelope>`)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	err := DeleteRecording(context.Background(), client, server.URL, "rec_1")
	if err != nil {
		t.Fatalf("DeleteRecording failed: %v", err)
	}
}

func TestGetReplayURI(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetReplayUriResponse xmlns="http://www.onvif.org/ver10/replay/wsdl">
      <Uri>rtsp://192.168.1.100:554/playback/rec_1</Uri>
    </GetReplayUriResponse>
  </s:Body>
</s:Envelope>`

	server := recordingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	uri, err := GetReplayURI(context.Background(), client, server.URL, "rec_1", "profile_1")
	if err != nil {
		t.Fatalf("GetReplayURI failed: %v", err)
	}

	if uri != "rtsp://192.168.1.100:554/playback/rec_1" {
		t.Errorf("unexpected replay URI: %s", uri)
	}
}

func TestGetRecordingConfiguration(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetRecordingConfigurationResponse xmlns="http://www.onvif.org/ver10/recording/wsdl">
      <RecordingConfiguration>
        <Source>vs_1</Source>
        <Content>Video</Content>
      </RecordingConfiguration>
    </GetRecordingConfigurationResponse>
  </s:Body>
</s:Envelope>`

	server := recordingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	config, err := GetRecordingConfiguration(context.Background(), client, server.URL, "rec_1")
	if err != nil {
		t.Fatalf("GetRecordingConfiguration failed: %v", err)
	}

	if config.Source != "vs_1" {
		t.Errorf("expected vs_1, got %s", config.Source)
	}
}

func TestGetRecordingTracks(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetTrackInfoResponse xmlns="http://www.onvif.org/ver10/recording/wsdl">
      <TrackInfo>
        <TrackToken>track_1</TrackToken>
        <TrackType>Video</TrackType>
        <Description>Main video track</Description>
      </TrackInfo>
    </GetTrackInfoResponse>
  </s:Body>
</s:Envelope>`

	server := recordingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	tracks, err := GetRecordingTracks(context.Background(), client, server.URL, "rec_1")
	if err != nil {
		t.Fatalf("GetRecordingTracks failed: %v", err)
	}

	if len(tracks) != 1 {
		t.Fatalf("expected 1 track, got %d", len(tracks))
	}

	if tracks[0].TrackType != "Video" {
		t.Errorf("expected Video, got %s", tracks[0].TrackType)
	}
}

func TestCreateRecordingJob(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <CreateRecordingJobResponse xmlns="http://www.onvif.org/ver10/recording/wsdl">
      <JobToken>job_1</JobToken>
    </CreateRecordingJobResponse>
  </s:Body>
</s:Envelope>`

	server := recordingTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	token, err := CreateRecordingJob(context.Background(), client, server.URL, "rec_1", "profile_1")
	if err != nil {
		t.Fatalf("CreateRecordingJob failed: %v", err)
	}

	if token != "job_1" {
		t.Errorf("expected job_1, got %s", token)
	}
}

func recordingTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}
