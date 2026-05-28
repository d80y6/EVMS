use ingest_service::api::{AppState, create_router, StreamInfo, StreamStatus};
use axum::{body::Body, http::{Request, StatusCode}};
use tower::ServiceExt;

#[tokio::test]
async fn test_health_endpoint() {
    let state = AppState::default();
    let app = create_router(state);
    
    let response = app
        .oneshot(Request::builder().uri("/health").body(Body::empty()).unwrap())
        .await
        .unwrap();
    
    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn test_metrics_endpoint() {
    let state = AppState::default();
    let app = create_router(state);
    
    let response = app
        .oneshot(Request::builder().uri("/metrics").body(Body::empty()).unwrap())
        .await
        .unwrap();
    
    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn test_streams_list_empty() {
    let state = AppState::default();
    let app = create_router(state);
    
    let response = app
        .oneshot(Request::builder().uri("/api/streams").body(Body::empty()).unwrap())
        .await
        .unwrap();
    
    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn test_stream_info_serialization() {
    let info = StreamInfo {
        stream_id: "test-stream".to_string(),
        status: StreamStatus::Active,
        started_at: Some(1234567890),
        video_codec: Some("H264".to_string()),
        audio_codec: Some("AAC".to_string()),
        bitrate_bps: Some(2500000),
        framerate: Some(30.0),
        resolution: Some((1920, 1080)),
        segment_count: 100,
        viewer_count: 5,
    };
    
    let json = serde_json::to_string(&info).unwrap();
    assert!(json.contains("test-stream"));
    assert!(json.contains("Active"));
    assert!(json.contains("H264"));
}

#[tokio::test]
async fn test_stream_status_transitions() {
    use std::fmt::{Display, Formatter};
    
    // Test Display impl for StreamStatus
    let active = StreamStatus::Active;
    let mut buf = String::new();
    write!(buf, "{}", active).unwrap();
    assert_eq!(buf, "active");
    
    let inactive = StreamStatus::Inactive;
    buf.clear();
    write!(buf, "{}", inactive).unwrap();
    assert_eq!(buf, "inactive");
    
    let error = StreamStatus::Error("test error".to_string());
    buf.clear();
    write!(buf, "{}", error).unwrap();
    assert!(buf.contains("error"));
}

#[tokio::test]
async fn test_webrtc_offer_endpoint() {
    let state = AppState::default();
    let app = create_router(state);
    
    let body = serde_json::json!({
        "streamId": "test-webrtc-stream"
    });
    
    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/api/webrtc/offer")
                .header("Content-Type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap()
        )
        .await
        .unwrap();
    
    // Should return OK or Bad Request depending on implementation
    assert!(response.status() == StatusCode::OK || response.status() == StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn test_webrtc_ice_endpoint() {
    let state = AppState::default();
    let app = create_router(state);
    
    let body = serde_json::json!({
        "streamId": "test-stream",
        "candidate": {
            "candidate": "a=candidate:1 1 udp 2130706431 192.168.1.1 50000 typ host",
            "sdpMid": "video",
            "sdpMLineIndex": 0
        }
    });
    
    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/api/webrtc/ice")
                .header("Content-Type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap()
        )
        .await
        .unwrap();
    
    // Should return OK or Bad Request
    assert!(response.status() == StatusCode::OK || response.status() == StatusCode::BAD_REQUEST);
}

#[tokio::test]
async fn test_graphql_endpoint_post() {
    let state = AppState::default();
    let app = create_router(state);
    
    let body = serde_json::json!({
        "query": "{ __typename }"
    });
    
    let response = app
        .oneshot(
            Request::builder()
                .method("POST")
                .uri("/graphql")
                .header("Content-Type", "application/json")
                .body(Body::from(serde_json::to_string(&body).unwrap()))
                .unwrap()
        )
        .await
        .unwrap();
    
    // GraphQL should respond with some JSON
    assert_eq!(response.status(), StatusCode::OK);
}

#[tokio::test]
async fn test_not_found() {
    let state = AppState::default();
    let app = create_router(state);
    
    let response = app
        .oneshot(Request::builder().uri("/nonexistent").body(Body::empty()).unwrap())
        .await
        .unwrap();
    
    assert_eq!(response.status(), StatusCode::NOT_FOUND);
}

#[test]
fn test_app_state_default() {
    let state = AppState::default();
    
    // Verify all subsystems are initialized
    assert!(state.rtsp_server.is_some());
    assert!(state.jitter_buffer_config.max_size > 0);
    assert!(state.muxer_config.segment_duration_ms > 0);
    assert!(state.storage_config.part_size_mb > 0);
}

#[test]
fn test_create_stream_info() {
    // Test creating StreamInfo with various states
    let active_info = StreamInfo {
        stream_id: "live-1".to_string(),
        status: StreamStatus::Active,
        started_at: Some(1000000),
        video_codec: Some("VP8".to_string()),
        audio_codec: None,
        bitrate_bps: None,
        framerate: Some(60.0),
        resolution: Some((1280, 720)),
        segment_count: 0,
        viewer_count: 0,
    };
    
    assert_eq!(active_info.stream_id, "live-1");
    assert!(matches!(active_info.status, StreamStatus::Active));
    assert_eq!(active_info.video_codec, Some("VP8".to_string()));
    assert_eq!(active_info.audio_codec, None);
}
