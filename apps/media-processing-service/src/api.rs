use axum::{Router, routing::{get, post, delete}, extract::{Path, State, Json}, response::IntoResponse};
use serde::{Deserialize, Serialize};
use tokio::sync::mpsc;
use tower_http::cors::{CorsLayer, Any};
use crate::config::Config;
use crate::pipeline::{PipelineManager, PipelineConfig, PipelineCommand, Pipeline};
use crate::preprocess::PreprocessConfig;
use crate::metrics::get_metrics_handle;

#[derive(Clone)]
pub struct AppState {
    pub manager: PipelineManager,
}

pub async fn start_api_server(config: Config, mut cmd_rx: mpsc::Receiver<PipelineCommand>) -> Result<(), Box<dyn std::error::Error>> {
    let (manager_tx, _) = mpsc::channel(100);
    let manager = PipelineManager::new(config.clone(), manager_tx);
    
    let state = AppState { manager };

    let cors = CorsLayer::new()
        .allow_origin(Any)
        .allow_methods(Any)
        .allow_headers(Any);

    let app = Router::new()
        .route("/health", get(health_check))
        .route("/metrics", get(metrics_handler))
        .route("/pipelines", get(list_pipelines))
        .route("/pipelines", post(create_pipeline))
        .route("/pipelines/:id/start", post(start_pipeline))
        .route("/pipelines/:id/stop", post(stop_pipeline))
        .route("/pipelines/:id", delete(delete_pipeline))
        .layer(cors)
        .with_state(state);

    let addr = format!("{}:{}", config.http_host, config.http_port);
    let listener = tokio::net::TcpListener::bind(&addr).await?;
    tracing::info!("API server listening on {}", addr);
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

#[derive(Debug, Deserialize)]
pub struct CreatePipelineRequest {
    pub id: Option<String>,
    pub source_uri: String,
    pub model_name: String,
    pub confidence_threshold: Option<f32>,
    pub nms_threshold: Option<f32>,
    pub target_width: Option<u32>,
    pub target_height: Option<u32>,
}

#[derive(Debug, Serialize)]
pub struct CreatePipelineResponse {
    pub id: String,
}

#[derive(Debug, Serialize)]
pub struct PipelineStatus {
    pub id: String,
    pub state: String,
    pub frame_count: u64,
    pub error_count: u64,
}

async fn list_pipelines(State(state): State<AppState>) -> impl IntoResponse {
    let pipelines = state.manager.list_pipelines().await;
    let status_list: Vec<PipelineStatus> = pipelines.iter().map(|p| {
        let state_str = match &p.state {
            crate::pipeline::PipelineState::Created => "created",
            crate::pipeline::PipelineState::Running => "running",
            crate::pipeline::PipelineState::Stopped => "stopped",
            crate::pipeline::PipelineState::Error(_) => "error",
        }.to_string();
        PipelineStatus {
            id: p.config.id.clone(),
            state: state_str,
            frame_count: p.frame_count,
            error_count: p.error_count,
        }
    }).collect();
    Json(status_list)
}

async fn create_pipeline(
    State(state): State<AppState>,
    Json(req): Json<CreatePipelineRequest>,
) -> impl IntoResponse {
    let id = req.id.unwrap_or_else(|| uuid::Uuid::new_v4().to_string());
    let config = PipelineConfig {
        id: id.clone(),
        source_uri: req.source_uri,
        model_name: req.model_name,
        preprocess: PreprocessConfig {
            target_width: req.target_width.unwrap_or(640),
            target_height: req.target_height.unwrap_or(480),
            ..Default::default()
        },
        confidence_threshold: req.confidence_threshold.unwrap_or(0.5),
        nms_threshold: req.nms_threshold.unwrap_or(0.5),
    };

    match state.manager.create_pipeline(config).await {
        Ok(_) => Json(CreatePipelineResponse { id }),
        Err(e) => (axum::http::StatusCode::BAD_REQUEST, Json(serde_json::json!({ "error": e.to_string() }))),
    }
}

async fn start_pipeline(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> impl IntoResponse {
    match state.manager.start_pipeline(&id).await {
        Ok(_) => Json(serde_json::json!({ "status": "started" })),
        Err(e) => (axum::http::StatusCode::NOT_FOUND, Json(serde_json::json!({ "error": e.to_string() }))),
    }
}

async fn stop_pipeline(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> impl IntoResponse {
    match state.manager.stop_pipeline(&id).await {
        Ok(_) => Json(serde_json::json!({ "status": "stopped" })),
        Err(e) => (axum::http::StatusCode::NOT_FOUND, Json(serde_json::json!({ "error": e.to_string() }))),
    }
}

async fn delete_pipeline(
    State(state): State<AppState>,
    Path(id): Path<String>,
) -> impl IntoResponse {
    match state.manager.delete_pipeline(&id).await {
        Ok(_) => Json(serde_json::json!({ "status": "deleted" })),
        Err(e) => (axum::http::StatusCode::NOT_FOUND, Json(serde_json::json!({ "error": e.to_string() }))),
    }
}
