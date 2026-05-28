//! API module - REST and WebSocket endpoints for edge sync service

use axum::{
    extract::State,
    http::StatusCode,
    response::Json,
    routing::{get, post},
    Router,
};
use serde::{Deserialize, Serialize};
use std::sync::Arc;

/// Application state for API handlers
#[derive(Clone)]
pub struct ApiState {
    pub app_state: Arc<crate::AppState>,
}

impl ApiState {
    pub fn new(app_state: Arc<crate::AppState>) -> Self {
        Self { app_state }
    }
}

/// Create the API router
pub fn create_router(state: ApiState) -> Router {
    Router::new()
        .route("/health", get(health_check))
        .route("/metrics", get(get_metrics))
        .route("/status", get(get_status))
        .route("/sync", post(trigger_sync))
        .route("/queue", get(get_queue_status))
        .route("/queue/purge", post(purge_queue))
        .route("/peers", get(get_peers))
        .route("/peers", post(register_peer))
        .route("/conflicts", get(get_conflicts))
        .route("/conflicts/:id/resolve", post(resolve_conflict))
        .route("/replication/stats", get(get_replication_stats))
        .with_state(state)
}

/// Health check endpoint
async fn health_check() -> Json<HealthResponse> {
    Json(HealthResponse {
        status: "healthy".to_string(),
        timestamp: current_timestamp(),
    })
}

/// Get metrics endpoint
async fn get_metrics(State(state): State<ApiState>) -> Result<String, StatusCode> {
    // Export Prometheus-format metrics
    Ok(String::new())
}

/// Get service status
async fn get_status(State(state): State<ApiState>) -> Json<ServiceStatus> {
    let sync_stats = match state.app_state.sync_engine.stats().await {
        Ok(stats) => stats,
        Err(_) => return Json(ServiceStatus::error("Failed to get sync stats")),
    };

    let queue_stats = state.app_state.offline_queue.stats().await;

    Json(ServiceStatus {
        device_id: state.app_state.config.device_id.clone(),
        is_online: sync_stats.is_online,
        total_entries: sync_stats.total_entries,
        pending_sync: sync_stats.pending_sync_count,
        failed_sync: sync_stats.failed_sync_count,
        queue_utilization: sync_stats.queue_utilization,
        vector_clock: format!("{}", sync_stats.vector_clock),
        timestamp: current_timestamp(),
    })
}

/// Trigger manual sync
async fn trigger_sync(State(state): State<ApiState>) -> Json<SyncResult> {
    match state.app_state.sync_engine.force_sync().await {
        Ok(result) => Json(SyncResult {
            success: true,
            synced_count: result.success_count,
            failed_count: result.failure_count,
            conflicts: result.conflicts.len(),
            message: "Sync completed".to_string(),
        }),
        Err(e) => Json(SyncResult {
            success: false,
            synced_count: 0,
            failed_count: 0,
            conflicts: 0,
            message: e.to_string(),
        }),
    }
}

/// Get queue status
async fn get_queue_status(State(state): State<ApiState>) -> Json<QueueStatus> {
    let stats = state.app_state.offline_queue.stats().await;
    
    Json(QueueStatus {
        total: stats.total,
        pending: stats.pending,
        in_progress: stats.in_progress,
        completed: stats.completed,
        failed: stats.failed,
        max_size: stats.max_size,
        utilization: stats.utilization(),
    })
}

/// Purge completed queue items
async fn purge_queue(State(state): State<ApiState>) -> Json<PurgeResult> {
    match state.app_state.offline_queue.purge_completed().await {
        Ok(count) => Json(PurgeResult {
            success: true,
            purged_count: count,
            message: format!("Purged {} items", count),
        }),
        Err(e) => Json(PurgeResult {
            success: false,
            purged_count: 0,
            message: e.to_string(),
        }),
    }
}

/// Get known peers
async fn get_peers(State(_state): State<ApiState>) -> Json<Vec<PeerInfo>> {
    // TODO: Implement peer listing
    Json(vec![])
}

/// Register a new peer
async fn register_peer(
    State(_state): State<ApiState>,
    Json(payload): Json<RegisterPeerRequest>,
) -> Json<PeerResult> {
    // TODO: Implement peer registration
    Json(PeerResult {
        success: true,
        message: format!("Peer {} registered", payload.peer_id),
    })
}

/// Get pending conflicts
async fn get_conflicts(State(_state): State<ApiState>) -> Json<Vec<ConflictInfo>> {
    // TODO: Implement conflict listing
    Json(vec![])
}

/// Resolve a conflict
async fn resolve_conflict(
    State(_state): State<ApiState>,
    path: axum::extract::Path<String>,
    Json(_payload): Json<ResolveConflictRequest>,
) -> Json<ResolveResult> {
    let conflict_id = path.0;
    // TODO: Implement conflict resolution
    Json(ResolveResult {
        success: true,
        message: format!("Conflict {} resolved", conflict_id),
    })
}

/// Get replication statistics
async fn get_replication_stats(State(_state): State<ApiState>) -> Json<ReplicationStatsResponse> {
    // TODO: Implement replication stats
    Json(ReplicationStatsResponse {
        total_operations: 0,
        successful: 0,
        failed: 0,
        success_rate: 0.0,
        peer_count: 0,
    })
}

// Request/Response types

#[derive(Debug, Serialize, Deserialize)]
pub struct HealthResponse {
    pub status: String,
    pub timestamp: u64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ServiceStatus {
    pub device_id: String,
    pub is_online: bool,
    pub total_entries: u64,
    pub pending_sync: usize,
    pub failed_sync: usize,
    pub queue_utilization: f64,
    pub vector_clock: String,
    pub timestamp: u64,
}

impl ServiceStatus {
    pub fn error(message: &str) -> Self {
        Self {
            device_id: String::new(),
            is_online: false,
            total_entries: 0,
            pending_sync: 0,
            failed_sync: 0,
            queue_utilization: 0.0,
            vector_clock: String::new(),
            timestamp: current_timestamp(),
        }
    }
}

#[derive(Debug, Serialize, Deserialize)]
pub struct SyncResult {
    pub success: bool,
    pub synced_count: usize,
    pub failed_count: usize,
    pub conflicts: usize,
    pub message: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct QueueStatus {
    pub total: usize,
    pub pending: usize,
    pub in_progress: usize,
    pub completed: usize,
    pub failed: usize,
    pub max_size: usize,
    pub utilization: f64,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PurgeResult {
    pub success: bool,
    pub purged_count: usize,
    pub message: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PeerInfo {
    pub peer_id: String,
    pub endpoint: String,
    pub last_seen: u64,
    pub status: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct RegisterPeerRequest {
    pub peer_id: String,
    pub endpoint: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct PeerResult {
    pub success: bool,
    pub message: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ConflictInfo {
    pub id: String,
    pub key: String,
    pub created_at: u64,
    pub strategy: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ResolveConflictRequest {
    pub choice: String,
    pub merged_value: Option<Vec<u8>>,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ResolveResult {
    pub success: bool,
    pub message: String,
}

#[derive(Debug, Serialize, Deserialize)]
pub struct ReplicationStatsResponse {
    pub total_operations: usize,
    pub successful: usize,
    pub failed: usize,
    pub success_rate: f64,
    pub peer_count: usize,
}

fn current_timestamp() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap()
        .as_millis() as u64
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_health_response() {
        let response = HealthResponse {
            status: "healthy".to_string(),
            timestamp: 12345,
        };
        
        let json = serde_json::to_string(&response).unwrap();
        assert!(json.contains("healthy"));
        assert!(json.contains("12345"));
    }

    #[test]
    fn test_service_status_serialization() {
        let status = ServiceStatus {
            device_id: "device1".to_string(),
            is_online: true,
            total_entries: 100,
            pending_sync: 5,
            failed_sync: 1,
            queue_utilization: 0.05,
            vector_clock: "[device1:10]".to_string(),
            timestamp: 12345,
        };
        
        let json = serde_json::to_string(&status).unwrap();
        assert!(json.contains("device1"));
        assert!(json.contains("true"));
    }
}
