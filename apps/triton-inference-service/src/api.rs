use axum::{
    extract::State,
    routing::{get, post},
    Json, Router,
    response::IntoResponse,
    http::StatusCode,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use crate::{InferenceState, InferenceRequest, InferenceResponse, error::Error};

/// REST API endpoints for inference service
pub fn create_router(state: Arc<InferenceState>) -> Router {
    Router::new()
        .route("/health", get(health_check))
        .route("/metrics", get(metrics_handler))
        .route("/infer", post(infer_handler))
        .route("/models", get(list_models))
        .route("/models/:model_name", get(get_model))
        .with_state(state)
}

async fn health_check() -> impl IntoResponse {
    Json(serde_json::json!({
        "status": "healthy",
        "service": "triton-inference"
    }))
}

async fn metrics_handler() -> impl IntoResponse {
    // In production, this would scrape Prometheus metrics
    "Metrics endpoint - scrape from /metrics Prometheus endpoint".into_response()
}

#[derive(Debug, Deserialize)]
pub struct InferRequest {
    pub model_name: String,
    pub inputs: Vec<crate::InputTensor>,
    #[serde(default)]
    pub parameters: crate::InferenceParameters,
}

async fn infer_handler(
    State(state): State<Arc<InferenceState>>,
    Json(req): Json<InferRequest>,
) -> Result<Json<InferenceResponse>> {
    let request_id = uuid::Uuid::new_v4().to_string();
    
    let inference_req = InferenceRequest {
        id: request_id,
        model_name: req.model_name.clone(),
        model_version: None,
        inputs: req.inputs,
        parameters: req.parameters,
    };

    // Acquire scheduler permit
    let _permit = state.scheduler.acquire().await?;
    
    // Add to batcher
    state.batcher.add(inference_req.clone()).await?;
    
    // Get batch and process
    let batch = state.batcher.get_batch().await?;
    
    if batch.is_empty() {
        return Err(AppError::Internal("Batch processing failed".to_string()));
    }

    // Perform inference (simplified - in production would call Triton)
    let response = state.client.infer(inference_req).await?;
    
    // Record metrics
    crate::metrics::increment_request_counter("success");
    crate::metrics::increment_model_inference(&req.model_name);
    
    Ok(Json(response))
}

#[derive(Debug, Serialize)]
pub struct ModelInfo {
    pub name: String,
    pub versions: Vec<String>,
    pub ready: bool,
}

async fn list_models(
    State(state): State<Arc<InferenceState>>,
) -> Result<Json<Vec<ModelInfo>>> {
    let models = state.models.list_all().await;
    let model_infos: Vec<ModelInfo> = models.iter().map(|m| ModelInfo {
        name: m.name.clone(),
        versions: m.versions.clone(),
        ready: m.ready,
    }).collect();
    
    Ok(Json(model_infos))
}

async fn get_model(
    State(state): State<Arc<InferenceState>>,
    axum::extract::Path(model_name): axum::extract::Path<String>,
) -> Result<Json<ModelInfo>> {
    let model = state.models.get(&model_name).await
        .ok_or_else(|| AppError::NotFound(format!("Model {} not found", model_name)))?;
    
    Ok(Json(ModelInfo {
        name: model.name.clone(),
        versions: model.versions.clone(),
        ready: model.ready,
    }))
}

#[derive(Debug, thiserror::Error)]
pub enum AppError {
    #[error("Internal error: {0}")]
    Internal(String),
    #[error("Not found: {0}")]
    NotFound(String),
    #[error("Bad request: {0}")]
    BadRequest(String),
}

impl From<Error> for AppError {
    fn from(err: Error) -> Self {
        match err {
            Error::InvalidInput(msg) => AppError::BadRequest(msg),
            Error::Model(msg) => AppError::NotFound(msg),
            _ => AppError::Internal(err.to_string()),
        }
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        let status = match &self {
            AppError::BadRequest(_) => StatusCode::BAD_REQUEST,
            AppError::NotFound(_) => StatusCode::NOT_FOUND,
            AppError::Internal(_) => StatusCode::INTERNAL_SERVER_ERROR,
        };
        
        let body = Json(serde_json::json!({
            "error": self.to_string()
        }));
        
        (status, body).into_response()
    }
}

pub type Result<T> = std::result::Result<T, AppError>;
