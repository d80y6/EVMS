//! Sync engine - core synchronization logic

use crate::{Conflict, ConflictResolutionStrategy, DataEntry, EdgeSyncError, OfflineQueue, QueueOperation, Result, StorageBackend, VectorClock};
use std::sync::Arc;
use tokio::sync::RwLock;
use tracing::{info, warn, error};

/// Synchronization engine
pub struct SyncEngine {
    storage: Arc<dyn StorageBackend + Send + Sync>,
    offline_queue: Arc<OfflineQueue>,
    conflict_resolver: Arc<crate::ConflictResolver>,
    device_id: String,
    local_clock: RwLock<VectorClock>,
    is_online: RwLock<bool>,
}

impl SyncEngine {
    pub fn new(
        storage: Arc<dyn StorageBackend + Send + Sync>,
        offline_queue: Arc<OfflineQueue>,
        conflict_resolver: Arc<crate::ConflictResolver>,
        device_id: String,
    ) -> Self {
        Self {
            storage,
            offline_queue,
            conflict_resolver,
            device_id,
            local_clock: RwLock::new(VectorClock::new()),
            is_online: RwLock::new(false),
        }
    }

    /// Set online/offline status
    pub async fn set_online(&self, online: bool) {
        let mut is_online = self.is_online.write().await;
        *is_online = online;
        
        if online {
            info!("Sync engine is now online");
        } else {
            warn!("Sync engine is now offline");
        }
    }

    /// Check if currently online
    pub async fn is_online(&self) -> bool {
        *self.is_online.read().await
    }

    /// Put a data entry (local operation)
    pub async fn put(&self, entry: DataEntry) -> Result<()> {
        // Increment local clock
        {
            let mut clock = self.local_clock.write().await;
            clock.increment(&self.device_id);
        }

        if self.is_online().await {
            // Store locally and sync immediately
            self.storage.put(entry.clone()).await?;
            self.sync_entry(entry).await?;
        } else {
            // Queue for later sync
            self.offline_queue.enqueue(entry, QueueOperation::Put(entry)).await?;
        }

        Ok(())
    }

    /// Delete a data entry (local operation)
    pub async fn delete(&self, key: &str) -> Result<()> {
        // Increment local clock
        {
            let mut clock = self.local_clock.write().await;
            clock.increment(&self.device_id);
        }

        if self.is_online().await {
            // Delete locally and sync
            if let Some(mut entry) = self.storage.get(key).await? {
                entry.deleted = true;
                self.storage.put(entry.clone()).await?;
                self.sync_entry(entry).await?;
            }
        } else {
            // Queue for later sync
            if let Some(entry) = self.storage.get(key).await? {
                self.offline_queue.enqueue(entry, QueueOperation::Delete(key.to_string())).await?;
            }
        }

        Ok(())
    }

    /// Get a data entry
    pub async fn get(&self, key: &str) -> Result<Option<DataEntry>> {
        self.storage.get(key).await
    }

    /// Sync a single entry to remote
    async fn sync_entry(&self, entry: DataEntry) -> Result<()> {
        // In a full implementation, this would send to central server
        // For now, just store locally
        info!("Syncing entry: {}", entry.key);
        Ok(())
    }

    /// Process pending queue items
    pub async fn process_queue(&self) -> Result<ProcessQueueResult> {
        let pending = self.offline_queue.get_pending().await;
        let mut success_count = 0;
        let mut failure_count = 0;
        let mut conflicts = Vec::new();

        for item in pending {
            match self.process_queue_item(&item).await {
                Ok(_) => {
                    self.offline_queue.update_status(&item.id, crate::QueueItemStatus::Completed).await?;
                    success_count += 1;
                }
                Err(e) => {
                    self.offline_queue.mark_failed(&item.id, e.to_string()).await?;
                    failure_count += 1;
                    
                    if let EdgeSyncError::ConflictError(_) = e {
                        if let Some(conflict) = self.extract_conflict(&item, &e) {
                            conflicts.push(conflict);
                        }
                    }
                }
            }
        }

        // Purge completed items
        self.offline_queue.purge_completed().await?;

        Ok(ProcessQueueResult {
            success_count,
            failure_count,
            conflicts,
        })
    }

    /// Process a single queue item
    async fn process_queue_item(&self, item: &crate::QueueItem) -> Result<()> {
        match &item.operation {
            QueueOperation::Put(entry) => {
                // Check for conflicts
                if let Some(existing) = self.storage.get(&entry.key).await? {
                    if existing.device_id != self.device_id && !existing.deleted {
                        // Potential conflict
                        let conflict = self.conflict_resolver.create_conflict(
                            &existing,
                            entry,
                            None,
                        );
                        
                        let resolution = self.conflict_resolver.resolve(&conflict)?;
                        self.storage.put(resolution.resolved_entry).await?;
                    } else {
                        self.storage.put(entry.clone()).await?;
                    }
                } else {
                    self.storage.put(entry.clone()).await?;
                }
            }
            QueueOperation::Delete(key) => {
                self.storage.delete(key).await?;
            }
            QueueOperation::Sync(entries) => {
                for entry in entries {
                    self.storage.put(entry.clone()).await?;
                }
            }
        }

        Ok(())
    }

    fn extract_conflict(&self, _item: &crate::QueueItem, _error: &EdgeSyncError) -> Option<Conflict> {
        // Extract conflict from error if possible
        None
    }

    /// Get current vector clock
    pub async fn get_clock(&self) -> VectorClock {
        self.local_clock.read().await.clone()
    }

    /// Merge remote vector clock
    pub async fn merge_clock(&self, remote_clock: &VectorClock) {
        let mut clock = self.local_clock.write().await;
        clock.merge(remote_clock);
    }

    /// Get sync statistics
    pub async fn stats(&self) -> Result<SyncStats> {
        let storage_stats = self.storage.stats().await?;
        let queue_stats = self.offline_queue.stats().await;
        let clock = self.get_clock().await;

        Ok(SyncStats {
            total_entries: storage_stats.total_entries,
            total_size_bytes: storage_stats.total_size_bytes,
            pending_sync_count: queue_stats.pending,
            failed_sync_count: queue_stats.failed,
            queue_utilization: queue_stats.utilization(),
            vector_clock: clock,
            is_online: self.is_online().await,
        })
    }

    /// Force sync all pending items
    pub async fn force_sync(&self) -> Result<ProcessQueueResult> {
        self.set_online(true).await;
        let result = self.process_queue().await;
        self.set_online(false).await;
        result
    }
}

/// Result of processing queue
#[derive(Debug, Clone, Default)]
pub struct ProcessQueueResult {
    pub success_count: usize,
    pub failure_count: usize,
    pub conflicts: Vec<Conflict>,
}

/// Sync statistics
#[derive(Debug, Clone)]
pub struct SyncStats {
    pub total_entries: u64,
    pub total_size_bytes: u64,
    pub pending_sync_count: usize,
    pub failed_sync_count: usize,
    pub queue_utilization: f64,
    pub vector_clock: VectorClock,
    pub is_online: bool,
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::MemoryBackend;

    #[tokio::test]
    async fn test_put_offline() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = Arc::new(OfflineQueue::new(storage.clone(), 100, crate::RetryPolicy::default()));
        let resolver = Arc::new(crate::ConflictResolver::new(ConflictResolutionStrategy::LastWriteWins));
        
        let engine = SyncEngine::new(storage.clone(), queue.clone(), resolver.clone(), "device1".to_string());
        
        // Start offline
        engine.set_online(false).await;
        
        let entry = DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string());
        engine.put(entry.clone()).await.unwrap();
        
        // Should be in queue, not in storage
        assert!(engine.get("key1").await.unwrap().is_none());
        
        let pending = queue.get_pending().await;
        assert_eq!(pending.len(), 1);
    }

    #[tokio::test]
    async fn test_put_online() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = Arc::new(OfflineQueue::new(storage.clone(), 100, crate::RetryPolicy::default()));
        let resolver = Arc::new(crate::ConflictResolver::new(ConflictResolutionStrategy::LastWriteWins));
        
        let engine = SyncEngine::new(storage.clone(), queue.clone(), resolver.clone(), "device1".to_string());
        
        // Start online
        engine.set_online(true).await;
        
        let entry = DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string());
        engine.put(entry.clone()).await.unwrap();
        
        // Should be in storage
        let retrieved = engine.get("key1").await.unwrap();
        assert!(retrieved.is_some());
        assert_eq!(retrieved.unwrap().value, b"value1");
    }

    #[tokio::test]
    async fn test_sync_stats() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = Arc::new(OfflineQueue::new(storage.clone(), 100, crate::RetryPolicy::default()));
        let resolver = Arc::new(crate::ConflictResolver::new(ConflictResolutionStrategy::LastWriteWins));
        
        let engine = SyncEngine::new(storage.clone(), queue.clone(), resolver.clone(), "device1".to_string());
        engine.set_online(true).await;
        
        let entry = DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string());
        engine.put(entry).await.unwrap();
        
        let stats = engine.stats().await.unwrap();
        assert_eq!(stats.total_entries, 1);
        assert!(stats.is_online);
    }
}
