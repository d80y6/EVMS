package onvif

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetProfiles(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetProfilesResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <Profiles token="profile_1">
        <Name>MainStream</Name>
        <VideoSource token="vs_1"/>
        <VideoEncoderConfiguration token="enc_1">
          <Name>MainStream_Enc</Name>
          <Encoding>H264</Encoding>
          <Resolution><Width>1920</Width><Height>1080</Height></Resolution>
          <Quality>5</Quality>
          <FrameRateLimit>30</FrameRateLimit>
          <BitrateLimit>4096</BitrateLimit>
          <GovLength>60</GovLength>
        </VideoEncoderConfiguration>
      </Profiles>
      <Profiles token="profile_2">
        <Name>SubStream</Name>
        <VideoSource token="vs_1"/>
        <VideoEncoderConfiguration token="enc_2">
          <Name>SubStream_Enc</Name>
          <Encoding>H264</Encoding>
          <Resolution><Width>640</Width><Height>360</Height></Resolution>
          <Quality>3</Quality>
          <FrameRateLimit>15</FrameRateLimit>
          <BitrateLimit>1024</BitrateLimit>
          <GovLength>30</GovLength>
        </VideoEncoderConfiguration>
      </Profiles>
    </GetProfilesResponse>
  </s:Body>
</s:Envelope>`

	server := mediaTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	profiles, err := GetProfiles(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetProfiles failed: %v", err)
	}

	if len(profiles) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(profiles))
	}

	if profiles[0].Token != "profile_1" {
		t.Errorf("expected profile_1, got %s", profiles[0].Token)
	}
	if profiles[0].VideoEncoderConfiguration.Width != 1920 {
		t.Errorf("expected 1920, got %d", profiles[0].VideoEncoderConfiguration.Width)
	}
	if profiles[0].VideoEncoderConfiguration.Encoding != "H264" {
		t.Errorf("expected H264, got %s", profiles[0].VideoEncoderConfiguration.Encoding)
	}

	if profiles[1].Token != "profile_2" {
		t.Errorf("expected profile_2, got %s", profiles[1].Token)
	}
}

func TestGetStreamURI(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetStreamUriResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <MediaUri>
        <Uri>rtsp://192.168.1.100:554/stream1</Uri>
        <InvalidAfterConnect>false</InvalidAfterConnect>
        <InvalidAfterReboot>false</InvalidAfterReboot>
        <Timeout>PT10S</Timeout>
      </MediaUri>
    </GetStreamUriResponse>
  </s:Body>
</s:Envelope>`

	server := mediaTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	uri, err := GetStreamURI(context.Background(), client, server.URL, "profile_1", "RTSP")
	if err != nil {
		t.Fatalf("GetStreamURI failed: %v", err)
	}

	if uri.URI != "rtsp://192.168.1.100:554/stream1" {
		t.Errorf("expected rtsp://..., got %s", uri.URI)
	}
}

func TestGetSnapshotURI(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetSnapshotUriResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <Uri>
        <Uri>http://192.168.1.100:80/snapshot.jpg</Uri>
      </Uri>
    </GetSnapshotUriResponse>
  </s:Body>
</s:Envelope>`

	server := mediaTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	snapshotURI, err := GetSnapshotURI(context.Background(), client, server.URL, "profile_1")
	if err != nil {
		t.Fatalf("GetSnapshotURI failed: %v", err)
	}

	if snapshotURI != "http://192.168.1.100:80/snapshot.jpg" {
		t.Errorf("unexpected snapshot URI: %s", snapshotURI)
	}
}

func TestGetVideoSources(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetVideoSourcesResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <VideoSources token="vs_1">
        <Resolution><Width>1920</Width><Height>1080</Height></Resolution>
        <Framerate>30</Framerate>
      </VideoSources>
    </GetVideoSourcesResponse>
  </s:Body>
</s:Envelope>`

	server := mediaTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	sources, err := GetVideoSources(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetVideoSources failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	if sources[0].ResolutionWidth != 1920 {
		t.Errorf("expected 1920, got %d", sources[0].ResolutionWidth)
	}
}

func TestFindMainProfile(t *testing.T) {
	profiles := []Profile{
		{Token: "low", VideoEncoderConfiguration: &VideoEncoderConfig{Width: 640, Encoding: "H264"}},
		{Token: "high", VideoEncoderConfiguration: &VideoEncoderConfig{Width: 1920, Encoding: "H264"}},
	}

	main := FindMainProfile(profiles)
	if main == nil {
		t.Fatal("main profile should not be nil")
	}
	if main.Token != "high" {
		t.Errorf("expected high, got %s", main.Token)
	}
}

func TestFindMainProfileNoHD(t *testing.T) {
	profiles := []Profile{
		{Token: "only", VideoEncoderConfiguration: &VideoEncoderConfig{Width: 704, Encoding: "H264"}},
	}

	main := FindMainProfile(profiles)
	if main == nil {
		t.Fatal("main profile should not be nil")
	}
	if main.Token != "only" {
		t.Errorf("expected only, got %s", main.Token)
	}
}

func TestFindMainProfileEmpty(t *testing.T) {
	main := FindMainProfile(nil)
	if main != nil {
		t.Error("expected nil for empty profiles")
	}
}

func TestFindSubProfile(t *testing.T) {
	profiles := []Profile{
		{Token: "main", VideoEncoderConfiguration: &VideoEncoderConfig{Width: 1920}},
		{Token: "sub", VideoEncoderConfiguration: &VideoEncoderConfig{Width: 640}},
	}

	sub := FindSubProfile(profiles, "main")
	if sub == nil {
		t.Fatal("sub profile should not be nil")
	}
	if sub.Token != "sub" {
		t.Errorf("expected sub, got %s", sub.Token)
	}
}

func TestFindSubProfileNotFound(t *testing.T) {
	profiles := []Profile{
		{Token: "only", VideoEncoderConfiguration: &VideoEncoderConfig{Width: 1920}},
	}

	sub := FindSubProfile(profiles, "only")
	if sub != nil {
		t.Error("expected nil when no other profile exists")
	}
}

func TestGetAudioSources(t *testing.T) {
	resp := `<?xml version="1.0"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <GetAudioSourcesResponse xmlns="http://www.onvif.org/ver10/media/wsdl">
      <AudioSources token="as_1">
        <Channels>1</Channels>
      </AudioSources>
    </GetAudioSourcesResponse>
  </s:Body>
</s:Envelope>`

	server := mediaTestServer(resp)
	defer server.Close()

	client := NewSOAPClient(5*time.Second, nil)
	sources, err := GetAudioSources(context.Background(), client, server.URL)
	if err != nil {
		t.Fatalf("GetAudioSources failed: %v", err)
	}

	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}

	if sources[0].Token != "as_1" {
		t.Errorf("expected as_1, got %s", sources[0].Token)
	}
}

func mediaTestServer(response string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/soap+xml")
		w.Write([]byte(response))
	}))
}
