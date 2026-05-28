//! Replication module for edge-to-edge and edge-to-cloud synchronization

use crate::{DataEntry, EdgeSyncError, ReplicationLogEntry, Result, VectorClock};
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{info, warn};

/// Replication manager for handling data replication between nodes
pub struct ReplicationManager {
    local_device_id: String,
    known_peers: RwLock<std::collections::HashMap<String, PeerInfo>>,
    replication_log: Arc<RwLock<Vec<ReplicationLogEntry>>>,
    max_log_size: usize,
}

impl ReplicationManager {
    pub fn new(local_device_id: String, max_log_size: usize) -> Self {
        Self {
            local_device_id,
            known_peers: RwLock::new(std::collections::HashMap::new()),
            replication_log: Arc::new(RwLock::new(Vec::new())),
            max_log_size,
        }
    }

    /// Register a known peer
    pub async fn register_peer(&self, peer_id: String, endpoint: String) {
        let mut peers = self.known_peers.write().await;
        peers.insert(peer_id, PeerInfo {
            endpoint,
            last_seen: current_timestamp(),
            sync_status: SyncStatus::Unknown,
        });
        info!("Registered peer: {}", peer_id);
    }

    /// Unregister a peer
    pub async fn unregister_peer(&self, peer_id: &str) {
        let mut peers = self.known_peers.write().await;
        peers.remove(peer_id);
        info!("Unregistered peer: {}", peer_id);
    }

    /// Update peer status
    pub async fn update_peer_status(&self, peer_id: &str, status: SyncStatus) {
        let mut peers = self.known_peers.write().await;
        if let Some(peer) = peers.get_mut(peer_id) {
            peer.last_seen = current_timestamp();
            peer.sync_status = status;
        }
    }

    /// Get all known peers
    pub async fn get_peers(&self) -> Vec<(String, PeerInfo)> {
        let peers = self.known_peers.read().await;
        peers.iter().map(|(k, v)| (k.clone(), v.clone())).collect()
    }

    /// Record a replication operation
    pub async fn record_replication(
        &self,
        operation: &str,
        entry: &DataEntry,
        target_device: &str,
        success: bool,
        error_message: Option<String>,
    ) {
        let log_entry = ReplicationLogEntry {
            id: uuid::Uuid::new_v4().to_string(),
            operation: operation.to_string(),
            key: entry.key.clone(),
            timestamp: current_timestamp(),
            source_device: self.local_device_id.clone(),
            target_device: target_device.to_string(),
            success,
            error_message,
        };

        let mut log = self.replication_log.write().await;
        log.push(log_entry);
        
        // Trim log if too large
        if log.len() > self.max_log_size {
            let remove_count = log.len() - self.max_log_size;
            log.drain(0..remove_count);
        }
    }

    /// Get recent replication logs
    pub async fn get_recent_logs(&self, limit: usize) -> Vec<ReplicationLogEntry> {
        let log = self.replication_log.read().await;
        log.iter().rev().take(limit).cloned().collect()
    }

    /// Get replication statistics
    pub async fn get_stats(&self) -> ReplicationStats {
        let log = self.replication_log.read().await;
        let peers = self.known_peers.read().await;
        
        let total_operations = log.len();
        let successful = log.iter().filter(|e| e.success).count();
        let failed = total_operations - successful;
        
        let success_rate = if total_operations > 0 {
            successful as f64 / total_operations as f64
        } else {
            0.0
        };

        ReplicationStats {
            total_operations,
            successful,
            failed,
            success_rate,
            peer_count: peers.len(),
            local_device_id: self.local_device_id.clone(),
        }
    }

    /// Create a delta of changes since a given timestamp
    pub async fn create_delta(
        &self,
        entries: &[DataEntry],
        since: u64,
    ) -> Delta {
        let changed: Vec<_> = entries
            .iter()
            .filter(|e| e.updated_at >= since)
            .cloned()
            .collect();

        Delta {
            since,
            entries: changed,
            device_id: self.local_device_id.clone(),
            timestamp: current_timestamp(),
        }
    }

    /// Apply a delta from another device
    pub async fn apply_delta(
        &self,
        delta: &Delta,
        storage: &dyn crate::StorageBackend,
        conflict_resolver: &crate::ConflictResolver,
    ) -> Result<ApplyDeltaResult> {
        let mut applied = 0;
        let mut conflicts = 0;
        let mut errors = 0;

        for entry in &delta.entries {
            // Check for conflicts
            if let Some(existing) = storage.get(&entry.key).await? {
                if existing.device_id != delta.device_id && !existing.deleted {
                    // Potential conflict
                    let conflict = conflict_resolver.create_conflict(&existing, entry, None);
                    
                    match conflict_resolver.resolve(&conflict) {
                        Ok(resolution) => {
                            storage.put(resolution.resolved_entry).await?;
                            applied += 1;
                        }
                        Err(e) => {
                            warn!("Failed to resolve conflict: {}", e);
                            errors += 1;
                        }
                    }
                    conflicts += 1;
                } else {
                    // No conflict, apply directly
                    storage.put(entry.clone()).await?;
                    applied += 1;
                }
            } else {
                // New entry
                storage.put(entry.clone()).await?;
                applied += 1;
            }

            // Record replication
            self.record_replication("apply_delta", entry, &delta.device_id, true, None).await;
        }

        Ok(ApplyDeltaResult {
            applied,
            conflicts,
            errors,
        })
    }
}

/// Peer information
#[derive(Debug, Clone)]
pub struct PeerInfo {
    pub endpoint: String,
    pub last_seen: u64,
    pub sync_status: SyncStatus,
}

/// Sync status for a peer
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum SyncStatus {
    Unknown,
    InSync,
    Syncing,
    Behind,
    Error(String),
}

/// Delta representing changes since a point in time
#[derive(Debug, Clone)]
pub struct Delta {
    pub since: u64,
    pub entries: Vec<DataEntry>,
    pub device_id: String,
    pub timestamp: u64,
}

impl Delta {
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }

    pub fn size_bytes(&self) -> u64 {
        self.entries.iter().map(|e| e.value.len() as u64).sum()
    }
}

/// Result of applying a delta
#[derive(Debug, Clone)]
pub struct ApplyDeltaResult {
    pub applied: usize,
    pub conflicts: usize,
    pub errors: usize,
}

/// Replication statistics
#[derive(Debug, Clone)]
pub struct ReplicationStats {
    pub total_operations: usize,
    pub successful: usize,
    pub failed: usize,
    pub success_rate: f64,
    pub peer_count: usize,
    pub local_device_id: String,
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
    use crate::MemoryBackend;

    #[tokio::test]
    async fn test_peer_management() {
        let manager = ReplicationManager::new("device1".to_string(), 1000);
        
        manager.register_peer("device2".to_string(), "http://device2:8080".to_string()).await;
        manager.register_peer("device3".to_string(), "http://device3:8080".to_string()).await;
        
        let peers = manager.get_peers().await;
        assert_eq!(peers.len(), 2);
        
        manager.unregister_peer("device2").await;
        
        let peers = manager.get_peers().await;
        assert_eq!(peers.len(), 1);
    }

    #[tokio::test]
    async fn test_replication_logging() {
        let manager = ReplicationManager::new("device1".to_string(), 1000);
        
        let entry = DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string());
        
        manager.record_replication("put", &entry, "device2", true, None).await;
        manager.record_replication("delete", &entry, "device2", false, Some("Network error".to_string())).await;
        
        let logs = manager.get_recent_logs(10).await;
        assert_eq!(logs.len(), 2);
        
        let stats = manager.get_stats().await;
        assert_eq!(stats.total_operations, 2);
        assert_eq!(stats.successful, 1);
        assert_eq!(stats.failed, 1);
    }

    #[tokio::test]
    async fn test_delta_creation() {
        let manager = ReplicationManager::new("device1".to_string(), 1000);
        
        let entries = vec![
            DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string()),
            DataEntry::new("key2".to_string(), b"value2".to_vec(), "device1".to_string()),
        ];
        
        let now = current_timestamp();
        let delta = manager.create_delta(&entries, now - 1000).await;
        
        assert_eq!(delta.entries.len(), 2);
        assert!(!delta.is_empty());
    }

    #[tokio::test]
    async fn test_delta_application() {
        let manager = ReplicationManager::new("device1".to_string(), 1000);
        let storage = MemoryBackend::new();
        let resolver = crate::ConflictResolver::new(crate::ConflictResolutionStrategy::LastWriteWins);
        
        let delta = Delta {
            since: 0,
            entries: vec![
                DataEntry::new("key1".to_string(), b"value1".to_vec(), "device2".to_string()),
                DataEntry::new("key2".to_string(), b"value2".to_vec(), "device2".to_string()),
            ],
            device_id: "device2".to_string(),
            timestamp: current_timestamp(),
        };
        
        let result = manager.apply_delta(&delta, &storage, &resolver).await.unwrap();
        
        assert_eq!(result.applied, 2);
        assert_eq!(result.conflicts, 0);
        
        // Verify entries are in storage
        assert!(storage.get("key1").await.unwrap().is_some());
        assert!(storage.get("key2").await.unwrap().is_some());
    }
}
