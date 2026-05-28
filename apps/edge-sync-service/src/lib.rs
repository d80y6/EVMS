// Edge Sync Service - Library Root

pub mod config;
pub mod error;
pub mod metrics;
pub mod storage;
pub mod vector_clock;
pub mod crdt;
pub mod sync_engine;
pub mod offline_queue;
pub mod replication;
pub mod conflict_resolver;
pub mod api;
pub mod grpc;

pub use config::Config;
pub use error::{EdgeSyncError, Result};
pub use storage::StorageBackend;
pub use vector_clock::VectorClock;
pub use sync_engine::SyncEngine;
pub use offline_queue::OfflineQueue;
pub use conflict_resolver::ConflictResolver;

use dashmap::DashMap;
use parking_lot::RwLock;
use std::sync::Arc;
use tracing::info;

/// Main application state for edge sync service
#[derive(Clone)]
pub struct AppState {
    pub config: Arc<Config>,
    pub storage: Arc<dyn StorageBackend + Send + Sync>,
    pub sync_engine: Arc<SyncEngine>,
    pub offline_queue: Arc<OfflineQueue>,
    pub conflict_resolver: Arc<ConflictResolver>,
    pub device_clocks: Arc<DashMap<String, VectorClock>>,
    pub pending_conflicts: Arc<DashMap<String, Conflict>>,
    pub replication_log: Arc<RwLock<Vec<ReplicationLogEntry>>>,
}

impl AppState {
    pub async fn new(config: Config) -> Result<Self> {
        let storage = Arc::new(storage::RocksDBBackend::new(&config.storage_path)?);
        let conflict_resolver = Arc::new(ConflictResolver::new(config.conflict_strategy));
        let offline_queue = Arc::new(OfflineQueue::new(
            storage.clone(),
            config.max_queue_size,
            config.retry_policy.clone(),
        ));
        let sync_engine = Arc::new(SyncEngine::new(
            storage.clone(),
            offline_queue.clone(),
            conflict_resolver.clone(),
            config.device_id.clone(),
        ));

        Ok(Self {
            config: Arc::new(config),
            storage,
            sync_engine,
            offline_queue,
            conflict_resolver,
            device_clocks: Arc::new(DashMap::new()),
            pending_conflicts: Arc::new(DashMap::new()),
            replication_log: Arc::new(RwLock::new(Vec::new())),
        })
    }

    pub fn record_replication(&self, entry: ReplicationLogEntry) {
        let mut log = self.replication_log.write();
        log.push(entry);
        if log.len() > self.config.max_replication_log_size {
            log.remove(0);
        }
    }
}

/// Replication log entry for audit trail
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct ReplicationLogEntry {
    pub id: String,
    pub operation: String,
    pub key: String,
    pub timestamp: u64,
    pub source_device: String,
    pub target_device: String,
    pub success: bool,
    pub error_message: Option<String>,
}

/// Conflict representation
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct Conflict {
    pub id: String,
    pub key: String,
    pub local_entry: DataEntry,
    pub remote_entry: DataEntry,
    pub strategy: ConflictResolutionStrategy,
    pub created_at: u64,
}

/// Data entry for synchronization
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct DataEntry {
    pub key: String,
    pub value: Vec<u8>,
    pub content_type: String,
    pub created_at: u64,
    pub updated_at: u64,
    pub metadata: std::collections::HashMap<String, String>,
    pub deleted: bool,
    pub version: u64,
    pub device_id: String,
}

impl DataEntry {
    pub fn new(key: String, value: Vec<u8>, device_id: String) -> Self {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        Self {
            key,
            value,
            content_type: "application/octet-stream".to_string(),
            created_at: now,
            updated_at: now,
            metadata: std::collections::HashMap::new(),
            deleted: false,
            version: 1,
            device_id,
        }
    }

    pub fn with_metadata(mut self, key: String, value: String) -> Self {
        self.metadata.insert(key, value);
        self
    }

    pub fn with_content_type(mut self, content_type: String) -> Self {
        self.content_type = content_type;
        self
    }
}

/// Conflict resolution strategies
#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub enum ConflictResolutionStrategy {
    LastWriteWins,
    Merge,
    Manual,
    Custom,
}

impl Default for ConflictResolutionStrategy {
    fn default() -> Self {
        ConflictResolutionStrategy::LastWriteWins
    }
}

/// Sync modes
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SyncMode {
    Push,
    Pull,
    Full,
}

/// Queue item status
#[derive(Debug, Clone, Copy, PartialEq, Eq, serde::Serialize, serde::Deserialize)]
pub enum QueueItemStatus {
    Pending,
    InProgress,
    Completed,
    Failed,
}

/// Retry policy configuration
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct RetryPolicy {
    pub max_retries: u32,
    pub initial_delay_ms: u64,
    pub max_delay_ms: u64,
    pub multiplier: f64,
}

impl Default for RetryPolicy {
    fn default() -> Self {
        Self {
            max_retries: 5,
            initial_delay_ms: 1000,
            max_delay_ms: 60000,
            multiplier: 2.0,
        }
    }
}

impl RetryPolicy {
    pub fn get_delay(&self, retry_count: u32) -> u64 {
        let delay = self.initial_delay_ms as f64 * self.multiplier.powi(retry_count as i32);
        (delay.min(self.max_delay_ms as f64) as u64)
    }
}

/// Initialize the edge sync service
pub async fn init(config: Config) -> Result<AppState> {
    info!("Initializing edge sync service");
    let state = AppState::new(config).await?;
    info!("Edge sync service initialized successfully");
    Ok(state)
}
