use ingest_service::rtsp::{RtspSession, RtspMethod, SdpBuilder};
use std::net::SocketAddr;

#[test]
fn test_rtsp_method_parse() {
    assert_eq!(RtspMethod::parse("OPTIONS").unwrap(), RtspMethod::Options);
    assert_eq!(RtspMethod::parse("DESCRIBE").unwrap(), RtspMethod::Describe);
    assert_eq!(RtspMethod::parse("SETUP").unwrap(), RtspMethod::Setup);
    assert_eq!(RtspMethod::parse("PLAY").unwrap(), RtspMethod::Play);
    assert_eq!(RtspMethod::parse("TEARDOWN").unwrap(), RtspMethod::Teardown);
    assert_eq!(RtspMethod::parse("RECORD").unwrap(), RtspMethod::Record);
    assert!(RtspMethod::parse("INVALID").is_err());
}

#[test]
fn test_rtsp_method_serialize() {
    assert_eq!(RtspMethod::Options.as_str(), "OPTIONS");
    assert_eq!(RtspMethod::Describe.as_str(), "DESCRIBE");
    assert_eq!(RtspMethod::Setup.as_str(), "SETUP");
    assert_eq!(RtspMethod::Play.as_str(), "PLAY");
    assert_eq!(RtspMethod::Teardown.as_str(), "TEARDOWN");
    assert_eq!(RtspMethod::Record.as_str(), "RECORD");
}

#[test]
fn test_sdp_builder_video_only() {
    let sdp = SdpBuilder::new()
        .session("Test Stream", "Test session")
        .video(96, 90000, "H264", Some("profile-level-id=4d001e;packetization-mode=1"))
        .build();
    
    assert!(sdp.contains("v=0"));
    assert!(sdp.contains("o=-"));
    assert!(sdp.contains("s=Test Stream"));
    assert!(sdp.contains("m=video"));
    assert!(sdp.contains("a=rtpmap:96 H264/90000"));
    assert!(sdp.contains("a=fmtp:96 profile-level-id=4d001e;packetization-mode=1"));
}

#[test]
fn test_sdp_builder_audio_only() {
    let sdp = SdpBuilder::new()
        .session("Audio Stream", "Audio only")
        .audio(97, 48000, "OPUS", Some("minptime=10;useinbandfec=1"))
        .build();
    
    assert!(sdp.contains("m=audio"));
    assert!(sdp.contains("a=rtpmap:97 OPUS/48000"));
    assert!(sdp.contains("a=fmtp:97 minptime=10;useinbandfec=1"));
}

#[test]
fn test_sdp_builder_av_stream() {
    let sdp = SdpBuilder::new()
        .session("AV Stream", "Audio and video")
        .video(96, 90000, "H264", None)
        .audio(97, 48000, "AAC", None)
        .build();
    
    // Should have both media sections
    let video_idx = sdp.find("m=video").unwrap();
    let audio_idx = sdp.find("m=audio").unwrap();
    assert!(video_idx > 0);
    assert!(audio_idx > 0);
}

#[test]
fn test_sdp_builder_with_control() {
    let sdp = SdpBuilder::new()
        .session("Stream", "With control URLs")
        .video(96, 90000, "H264", None)
        .control_track("trackID=1", "rtsp://example.com/stream/trackID=1")
        .build();
    
    assert!(sdp.contains("a=control:trackID=1"));
    assert!(sdp.contains("rtsp://example.com/stream/trackID=1"));
}

#[test]
fn test_rtsp_session_creation() {
    let addr: SocketAddr = "127.0.0.1:554".parse().unwrap();
    let session = RtspSession::new(addr, "test-stream");
    
    assert_eq!(session.stream_id, "test-stream");
    assert_eq!(session.remote_addr, addr);
    assert!(session.session_id.is_empty()); // Not started yet
}

#[test]
fn test_rtsp_response_options() {
    let response = ingest_service::rtsp::build_options_response(1, "CSeq");
    
    assert!(response.contains("RTSP/1.0 200 OK"));
    assert!(response.contains("CSeq: 1"));
    assert!(response.contains("Public: OPTIONS, DESCRIBE, SETUP, TEARDOWN, PLAY, PAUSE"));
}

#[test]
fn test_rtsp_response_describe() {
    let sdp = "v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\n";
    let response = ingest_service::rtsp::build_describe_response(1, "CSeq", sdp, "application/sdp");
    
    assert!(response.contains("RTSP/1.0 200 OK"));
    assert!(response.contains("CSeq: 1"));
    assert!(response.contains("Content-Type: application/sdp"));
    assert!(response.contains("Content-Length:"));
    assert!(response.contains(sdp));
}

#[test]
fn test_rtsp_response_setup() {
    let response = ingest_service::rtsp::build_setup_response(
        1, 
        "CSeq", 
        "test-session-id",
        ("127.0.0.1", 50000),
        ("127.0.0.1", 50001),
    );
    
    assert!(response.contains("RTSP/1.0 200 OK"));
    assert!(response.contains("CSeq: 1"));
    assert!(response.contains("Session: test-session-id"));
    assert!(response.contains("Transport: RTP/AVP;unicast;client_port=50000-50001"));
}

#[test]
fn test_rtsp_response_play() {
    let response = ingest_service::rtsp::build_play_response(1, "CSeq", "test-session-id", Some(0));
    
    assert!(response.contains("RTSP/1.0 200 OK"));
    assert!(response.contains("CSeq: 1"));
    assert!(response.contains("Session: test-session-id"));
    assert!(response.contains("Range: npt=0.000-"));
}

#[test]
fn test_rtsp_response_teardown() {
    let response = ingest_service::rtsp::build_teardown_response(1, "CSeq", "test-session-id");
    
    assert!(response.contains("RTSP/1.0 200 OK"));
    assert!(response.contains("CSeq: 1"));
    assert!(response.contains("Session: test-session-id"));
}

#[test]
fn test_rtsp_response_error() {
    let response = ingest_service::rtsp::build_error_response(1, "CSeq", 400, "Bad Request");
    
    assert!(response.contains("RTSP/1.0 400 Bad Request"));
    assert!(response.contains("CSeq: 1"));
}

#[test]
fn test_parse_rtsp_request() {
    let request = "DESCRIBE rtsp://example.com/stream RTSP/1.0\r\nCSeq: 1\r\nAccept: application/sdp\r\n\r\n";
    let (method, uri, version, headers) = ingest_service::rtsp::parse_request_line_and_headers(request).unwrap();
    
    assert_eq!(method, "DESCRIBE");
    assert_eq!(uri, "rtsp://example.com/stream");
    assert_eq!(version, "RTSP/1.0");
    assert_eq!(headers.get("CSeq"), Some(&"1".to_string()));
    assert_eq!(headers.get("Accept"), Some(&"application/sdp".to_string()));
}

#[tokio::test]
async fn test_session_state_transitions() {
    use ingest_service::rtsp::SessionState;
    
    let mut state = SessionState::Initial;
    assert!(matches!(state, SessionState::Initial));
    
    state = SessionState::Ready;
    assert!(matches!(state, SessionState::Ready));
    
    state = SessionState::Playing { start_time: std::time::Instant::now() };
    assert!(matches!(state, SessionState::Playing { .. }));
    
    state = SessionState::Paused;
    assert!(matches!(state, SessionState::Paused));
    
    state = SessionState::Terminated;
    assert!(matches!(state, SessionState::Terminated));
}
