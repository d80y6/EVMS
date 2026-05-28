//! API server with GraphQL and REST endpoints

use axum::{
    extract::{Path, State},
    http::StatusCode,
    response::Json,
    routing::{get, post},
    Router,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use tower_http::trace::TraceLayer;
use tracing::info;

use crate::error::Result;
use crate::AppState;

/// API state wrapper
#[derive(Clone)]
pub struct ApiState {
    pub app_state: Arc<AppState>,
}

/// Health check response
#[derive(Debug, Serialize, Deserialize)]
pub struct HealthResponse {
    pub status: String,
    pub version: String,
    pub uptime_secs: u64,
    pub active_sessions: usize,
    pub active_peers: usize,
}

/// Stream stats response
#[derive(Debug, Serialize, Deserialize)]
pub struct StreamStatsResponse {
    pub ssrc: u32,
    pub packets_received: u64,
    pub packets_lost: u64,
    pub jitter_ms: f64,
    pub bitrate_bps: u64,
}

/// Create offer request
#[derive(Debug, Serialize, Deserialize)]
pub struct CreateOfferRequest {
    pub stream_id: Option<String>,
}

/// Create offer response
#[derive(Debug, Serialize, Deserialize)]
pub struct CreateOfferResponse {
    pub peer_id: String,
    pub sdp: String,
}

/// Answer request
#[derive(Debug, Serialize, Deserialize)]
pub struct AnswerRequest {
    pub peer_id: String,
    pub sdp: String,
}

/// ICE candidate request
#[derive(Debug, Serialize, Deserialize)]
pub struct IceCandidateRequest {
    pub peer_id: String,
    pub candidate: String,
    pub sdp_mid: String,
    pub sdp_mline_index: u16,
}

/// Build the API router
pub fn create_router(state: ApiState) -> Router {
    Router::new()
        .route("/health", get(health_check))
        .route("/metrics", get(metrics_handler))
        .route("/streams", get(list_streams))
        .route("/streams/:stream_id", get(get_stream_stats))
        .route("/webrtc/offer", post(create_offer))
        .route("/webrtc/answer", post(handle_answer))
        .route("/webrtc/ice", post(add_ice_candidate))
        .layer(TraceLayer::new_for_http())
        .with_state(state)
}

/// Run the API server
pub async fn run_server(state: Arc<AppState>) -> Result<()> {
    let api_state = ApiState {
        app_state: Arc::clone(&state),
    };
    
    let router = create_router(api_state);
    
    let listener = tokio::net::TcpListener::bind(&state.config.api_bind_addr)
        .await
        .map_err(|e| crate::Error::Network(e))?;
    
    info!("API server listening on {}", state.config.api_bind_addr);
    
    axum::serve(listener, router)
        .await
        .map_err(|e| crate::Error::Network(e))?;
    
    Ok(())
}

/// Health check handler
async fn health_check(State(state): State<ApiState>) -> Json<HealthResponse> {
    let rtsp_sessions = state.app_state.rtsp_manager.session_count();
    let webrtc_peers = state.app_state.webrtc.signaling.active_peer_count();
    
    Json(HealthResponse {
        status: "healthy".to_string(),
        version: env!("CARGO_PKG_VERSION").to_string(),
        uptime_secs: 0, // Would track from start time
        active_sessions: rtsp_sessions,
        active_peers: webrtc_peers,
    })
}

/// Metrics handler (proxies to Prometheus)
async fn metrics_handler(State(state): State<ApiState>) -> String {
    state.app_state.metrics_handle.recorder_handle.render()
}

/// List active streams
async fn list_streams(State(state): State<ApiState>) -> Json<Vec<StreamInfo>> {
    let sessions = state.app_state.rtsp_manager.session_count();
    
    // Return simplified stream list
    Json(vec![])
}

#[derive(Debug, Serialize, Deserialize)]
pub struct StreamInfo {
    pub id: String,
    pub ssrc: u32,
    pub created_at: String,
}

/// Get stream statistics
async fn get_stream_stats(
    State(state): State<ApiState>,
    Path(stream_id): Path<String>,
) -> Result<Json<StreamStatsResponse>> {
    // Parse SSRC from stream_id or look it up
    let ssrc: u32 = stream_id.parse().unwrap_or(0);
    
    // Get RTP stats
    let rtp_stats = state.app_state.rtp_buffer.get_stats(ssrc);
    
    // Get RTCP stats
    let rtcp_stats = state.app_state.rtcp_handler.get_stats(ssrc);
    
    let stats = StreamStatsResponse {
        ssrc,
        packets_received: rtp_stats.as_ref().map(|s| s.packet_count as u64).unwrap_or(0),
        packets_lost: rtp_stats.as_ref().map(|s| s.loss_count).unwrap_or(
            rtcp_stats.as_ref().map(|s| s.packets_lost).unwrap_or(0)
        ),
        jitter_ms: rtcp_stats.as_ref().map(|s| s.jitter_ms).unwrap_or(0.0),
        bitrate_bps: 0, // Would calculate from recent data
    };
    
    Ok(Json(stats))
}

/// Create WebRTC offer
async fn create_offer(
    State(state): State<ApiState>,
    Json(req): Json<CreateOfferRequest>,
) -> Result<Json<CreateOfferResponse>> {
    let peer_id = state.app_state.webrtc.signaling.create_peer();
    
    let offer = state.app_state.webrtc.signaling.create_offer(&peer_id)?;
    
    Ok(Json(CreateOfferResponse {
        peer_id,
        sdp: offer.sdp,
    }))
}

/// Handle WebRTC answer
async fn handle_answer(
    State(state): State<ApiState>,
    Json(req): Json<AnswerRequest>,
) -> Result<StatusCode> {
    state.app_state.webrtc.signaling.handle_offer(&req.peer_id, req.sdp)?;
    
    Ok(StatusCode::OK)
}

/// Add ICE candidate
async fn add_ice_candidate(
    State(state): State<ApiState>,
    Json(req): Json<IceCandidateRequest>,
) -> Result<StatusCode> {
    use crate::webrtc::IceCandidate;
    
    let candidate = IceCandidate {
        candidate: req.candidate,
        sdp_mid: req.sdp_mid,
        sdp_mline_index: req.sdp_mline_index,
    };
    
    state.app_state.webrtc.signaling.add_ice_candidate(&req.peer_id, candidate)?;
    
    Ok(StatusCode::OK)
}

/// GraphQL schema (placeholder for full implementation)
pub mod graphql {
    use async_graphql::*;
    use crate::rtp::StreamStats;
    
    /// Query root
    pub struct QueryRoot;
    
    #[Object]
    impl QueryRoot {
        /// Get health status
        async fn health(&self) -> HealthStatus {
            HealthStatus {
                status: "healthy".to_string(),
            }
        }
        
        /// Get stream by SSRC
        async fn stream(&self, ssrc: u32) -> Option<Stream> {
            // Would query actual state
            Some(Stream {
                ssrc,
                packets_received: 0,
                packets_lost: 0,
            })
        }
    }
    
    /// Health status
    pub struct HealthStatus {
        pub status: String,
    }
    
    #[Object]
    impl HealthStatus {
        async fn status(&self) -> &str {
            &self.status
        }
    }
    
    /// Stream information
    pub struct Stream {
        pub ssrc: u32,
        pub packets_received: u64,
        pub packets_lost: u64,
    }
    
    #[Object]
    impl Stream {
        async fn ssrc(&self) -> u32 {
            self.ssrc
        }
        
        async fn packets_received(&self) -> u64 {
            self.packets_received
        }
        
        async fn packets_lost(&self) -> u64 {
            self.packets_lost
        }
    }
    
    /// Create schema
    pub fn create_schema() -> Schema<QueryRoot, EmptyMutation, EmptySubscription> {
        Schema::build(QueryRoot, EmptyMutation, EmptySubscription)
            .finish()
    }
}
