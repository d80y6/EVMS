use axum::{Router, routing::{get, post}, extract::{Path, State, Json}, response::IntoResponse};
use serde::{Deserialize, Serialize};
use tower_http::cors::{CorsLayer, Any};
use crate::config::Config;
use crate::clock::{TimeSync, HybridLogicalClock};
use crate::aligner::StreamAligner;
use crate::segment_manager::SegmentManager;
use crate::metrics::get_metrics_handle;

#[derive(Clone)]
pub struct AppState {
    pub time_sync: TimeSync,
    pub aligner: StreamAligner,
    pub segment_manager: SegmentManager,
}

pub async fn start_api_server(config: Config) -> Result<(), Box<dyn std::error::Error>> {
    let state = AppState {
        time_sync: TimeSync::new(),
        aligner: StreamAligner::new(),
        segment_manager: SegmentManager::new(),
    };

    let cors = CorsLayer::new()
        .allow_origin(Any)
        .allow_methods(Any)
        .allow_headers(Any);

    let app = Router::new()
        .route("/health", get(health_check))
        .route("/metrics", get(metrics_handler))
        .route("/sync/status", get(sync_status))
        .route("/sync/update", post(update_sync))
        .route("/align/plan", get(alignment_plan))
        .route("/align/set_offset", post(set_offset))
        .route("/segments", post(add_segment))
        .route("/segments/:stream_id", get(get_segments))
        .route("/virtual/create", post(create_virtual))
        .layer(cors)
        .with_state(state);

    let addr = format!("{}:{}", config.http_host, config.http_port);
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    tracing::info!("Timeline API server listening on {}", addr);
    axum::serve(listener, app).await?;
    Ok(())
}

async fn health_check() -> impl IntoResponse {
    Json(serde_json::json!({ "status": "healthy" }))
}

async fn metrics_handler() -> impl IntoResponse {
    let handle = get_metrics_handle();
    handle.render()
}

#[derive(Debug, Serialize)]
pub struct SyncStatus {
    pub clock_state: crate::clock::ClockState,
    pub offset_ms: f64,
    pub rtt_ms: f64,
}

async fn sync_status(State(state): State<AppState>) -> impl IntoResponse {
    let clock_state = state.time_sync.get_clock().get_state();
    Json(SyncStatus {
        clock_state,
        offset_ms: 0.0,
        rtt_ms: 0.0,
    })
}

#[derive(Debug, Deserialize)]
pub struct UpdateSyncRequest {
    pub offset_ms: f64,
    pub rtt_ms: f64,
}

async fn update_sync(
    State(mut state): State<AppState>,
    Json(req): Json<UpdateSyncRequest>,
) -> impl IntoResponse {
    state.time_sync.update_offset(req.offset_ms, req.rtt_ms);
    Json(serde_json::json!({ "status": "updated" }))
}

async fn alignment_plan(State(state): State<AppState>) -> impl IntoResponse {
    match state.aligner.get_alignment_plan() {
        Some(plan) => Json(plan),
        None => (axum::http::StatusCode::NOT_FOUND, Json(serde_json::json!({ "error": "No reference stream set" }))),
    }
}

#[derive(Debug, Deserialize)]
pub struct SetOffsetRequest {
    pub stream_id: String,
    pub offset_ms: f64,
}

async fn set_offset(
    State(mut state): State<AppState>,
    Json(req): Json<SetOffsetRequest>,
) -> impl IntoResponse {
    state.aligner.set_offset(&req.stream_id, req.offset_ms);
    Json(serde_json::json!({ "status": "ok" }))
}

#[derive(Debug, Deserialize)]
pub struct AddSegmentRequest {
    pub stream_id: String,
    pub start_time_ms: i64,
    pub end_time_ms: i64,
    pub uri: String,
}

async fn add_segment(
    State(mut state): State<AppState>,
    Json(req): Json<AddSegmentRequest>,
) -> impl IntoResponse {
    use chrono::Utc;
    use uuid::Uuid;
    
    let segment = crate::segment_manager::StreamSegment {
        id: Uuid::new_v4().to_string(),
        stream_id: req.stream_id.clone(),
        start_time_ms: req.start_time_ms,
        end_time_ms: req.end_time_ms,
        uri: req.uri,
        duration_ms: req.end_time_ms - req.start_time_ms,
        created_at: Utc::now(),
    };

    match state.segment_manager.add_segment(segment) {
        Ok(_) => Json(serde_json::json!({ "status": "created" })),
        Err(e) => (axum::http::StatusCode::BAD_REQUEST, Json(serde_json::json!({ "error": e }))),
    }
}

async fn get_segments(
    State(state): State<AppState>,
    Path(stream_id): Path<String>,
) -> impl IntoResponse {
    let segments = state.segment_manager.get_segments_by_stream(&stream_id);
    Json(segments)
}

#[derive(Debug, Deserialize)]
pub struct CreateVirtualRequest {
    pub stream_ids: Vec<String>,
    pub start_time_ms: i64,
    pub end_time_ms: i64,
}

async fn create_virtual(
    State(state): State<AppState>,
    Json(req): Json<CreateVirtualRequest>,
) -> impl IntoResponse {
    match state.segment_manager.create_virtual_segment(&req.stream_ids, req.start_time_ms, req.end_time_ms) {
        Some(virtual) => Json(virtual),
        None => (axum::http::StatusCode::NOT_FOUND, Json(serde_json::json!({ "error": "No segments found" }))),
    }
}
