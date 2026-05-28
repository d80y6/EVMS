use axum::{extract::State, routing::{get, post}, Json, Router, response::IntoResponse, http::StatusCode};
use serde::{Deserialize, Serialize};
use std::sync::Arc;
use crate::IndexState;

pub fn create_router(state: Arc<IndexState>) -> Router {
    Router::new()
        .route("/health", get(health_check))
        .route("/collections", get(list_collections).post(create_collection))
        .route("/collections/:name/search", post(search))
        .route("/collections/:name/insert", post(insert))
        .with_state(state)
}

async fn health_check() -> impl IntoResponse {
    Json(serde_json::json!({"status": "healthy", "service": "vector-index"}))
}

#[derive(Debug, Deserialize)]
pub struct CreateCollectionRequest {
    pub name: String,
    pub dimension: i64,
}

async fn create_collection(
    State(state): State<Arc<IndexState>>,
    Json(req): Json<CreateCollectionRequest>,
) -> Result<Json<serde_json::Value>, AppError> {
    state.milvus.create_collection(&req.name, req.dimension).await?;
    Ok(Json(serde_json::json!({"success": true, "collection": req.name})))
}

async fn list_collections(
    State(state): State<Arc<IndexState>>,
) -> Result<Json<Vec<String>>, AppError> {
    Ok(Json(vec![]))
}

#[derive(Debug, Deserialize)]
pub struct SearchRequest {
    pub query: Vec<f32>,
    pub k: usize,
}

async fn search(
    State(state): State<Arc<IndexState>>,
    axum::extract::Path(name): axum::extract::Path<String>,
    Json(req): Json<SearchRequest>,
) -> Result<Json<Vec<SearchResult>>, AppError> {
    let results = state.milvus.search(&name, &req.query, req.k as i64).await?;
    let api_results: Vec<SearchResult> = results.into_iter().map(|r| SearchResult {
        id: r.id,
        score: r.score,
    }).collect();
    Ok(Json(api_results))
}

#[derive(Debug, Deserialize)]
pub struct InsertRequest {
    pub vectors: Vec<Vec<f32>>,
    pub ids: Vec<i64>,
}

async fn insert(
    State(state): State<Arc<IndexState>>,
    axum::extract::Path(name): axum::extract::Path<String>,
    Json(req): Json<InsertRequest>,
) -> Result<Json<serde_json::Value>, AppError> {
    state.milvus.insert(&name, &req.vectors, &req.ids).await?;
    Ok(Json(serde_json::json!({"success": true, "count": req.vectors.len()})))
}

#[derive(Debug, Serialize)]
pub struct SearchResult {
    pub id: i64,
    pub score: f32,
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

impl From<crate::error::Error> for AppError {
    fn from(err: crate::error::Error) -> Self {
        AppError::Internal(err.to_string())
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> axum::response::Response {
        let status = match &self {
            AppError::BadRequest(_) => StatusCode::BAD_REQUEST,
            AppError::NotFound(_) => StatusCode::NOT_FOUND,
            AppError::Internal(_) => StatusCode::INTERNAL_SERVER_ERROR,
        };
        (status, Json(serde_json::json!({"error": self.to_string()}))).into_response()
    }
}

pub type Result<T> = std::result::Result<T, AppError>;
