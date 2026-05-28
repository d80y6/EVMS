//! Offline queue for storing operations when disconnected

use crate::{DataEntry, EdgeSyncError, QueueItemStatus, Result, RetryPolicy};
use std::sync::Arc;
use tokio::sync::RwLock;
use uuid::Uuid;

/// Queue item representing a pending operation
#[derive(Debug, Clone)]
pub struct QueueItem {
    pub id: String,
    pub entry: DataEntry,
    pub operation: QueueOperation,
    pub created_at: u64,
    pub retry_count: u32,
    pub status: QueueItemStatus,
    pub last_error: Option<String>,
}

impl QueueItem {
    pub fn new(entry: DataEntry, operation: QueueOperation) -> Self {
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        
        Self {
            id: Uuid::new_v4().to_string(),
            entry,
            operation,
            created_at: now,
            retry_count: 0,
            status: QueueItemStatus::Pending,
            last_error: None,
        }
    }

    pub fn can_retry(&self, policy: &RetryPolicy) -> bool {
        self.retry_count < policy.max_retries
    }

    pub fn mark_failed(&mut self, error: String) {
        self.retry_count += 1;
        self.last_error = Some(error);
        if !self.can_retry(&RetryPolicy::default()) {
            self.status = QueueItemStatus::Failed;
        } else {
            self.status = QueueItemStatus::Pending;
        }
    }

    pub fn mark_in_progress(&mut self) {
        self.status = QueueItemStatus::InProgress;
    }

    pub fn mark_completed(&mut self) {
        self.status = QueueItemStatus::Completed;
    }
}

/// Operation types for the queue
#[derive(Debug, Clone)]
pub enum QueueOperation {
    Put(DataEntry),
    Delete(String),
    Sync(Vec<DataEntry>),
}

/// Offline queue manager
pub struct OfflineQueue {
    items: RwLock<Vec<QueueItem>>,
    storage: Arc<dyn crate::StorageBackend + Send + Sync>,
    max_size: usize,
    retry_policy: RetryPolicy,
}

impl OfflineQueue {
    pub fn new(
        storage: Arc<dyn crate::StorageBackend + Send + Sync>,
        max_size: usize,
        retry_policy: RetryPolicy,
    ) -> Self {
        Self {
            items: RwLock::new(Vec::new()),
            storage,
            max_size,
            retry_policy,
        }
    }

    /// Add an item to the queue
    pub async fn enqueue(&self, entry: DataEntry, operation: QueueOperation) -> Result<String> {
        let mut items = self.items.write().await;
        
        if items.len() >= self.max_size {
            return Err(EdgeSyncError::QueueError("Queue is full".to_string()));
        }

        let item = QueueItem::new(entry, operation);
        let id = item.id.clone();
        items.push(item);
        
        Ok(id)
    }

    /// Get pending items
    pub async fn get_pending(&self) -> Vec<QueueItem> {
        let items = self.items.read().await;
        items.iter()
            .filter(|item| item.status == QueueItemStatus::Pending)
            .cloned()
            .collect()
    }

    /// Get failed items
    pub async fn get_failed(&self) -> Vec<QueueItem> {
        let items = self.items.read().await;
        items.iter()
            .filter(|item| item.status == QueueItemStatus::Failed)
            .cloned()
            .collect()
    }

    /// Get item by ID
    pub async fn get_by_id(&self, id: &str) -> Option<QueueItem> {
        let items = self.items.read().await;
        items.iter().find(|item| item.id == id).cloned()
    }

    /// Update item status
    pub async fn update_status(&self, id: &str, status: QueueItemStatus) -> Result<()> {
        let mut items = self.items.write().await;
        if let Some(item) = items.iter_mut().find(|i| i.id == id) {
            item.status = status;
            Ok(())
        } else {
            Err(EdgeSyncError::NotFound(format!("Queue item {} not found", id)))
        }
    }

    /// Mark item as failed with error
    pub async fn mark_failed(&self, id: &str, error: String) -> Result<()> {
        let mut items = self.items.write().await;
        if let Some(item) = items.iter_mut().find(|i| i.id == id) {
            item.mark_failed(error);
            Ok(())
        } else {
            Err(EdgeSyncError::NotFound(format!("Queue item {} not found", id)))
        }
    }

    /// Remove completed items
    pub async fn purge_completed(&self) -> Result<usize> {
        let mut items = self.items.write().await;
        let initial_len = items.len();
        items.retain(|item| item.status != QueueItemStatus::Completed);
        Ok(initial_len - items.len())
    }

    /// Get queue statistics
    pub async fn stats(&self) -> QueueStats {
        let items = self.items.read().await;
        let pending = items.iter().filter(|i| i.status == QueueItemStatus::Pending).count();
        let in_progress = items.iter().filter(|i| i.status == QueueItemStatus::InProgress).count();
        let completed = items.iter().filter(|i| i.status == QueueItemStatus::Completed).count();
        let failed = items.iter().filter(|i| i.status == QueueItemStatus::Failed).count();

        QueueStats {
            total: items.len(),
            pending,
            in_progress,
            completed,
            failed,
            max_size: self.max_size,
        }
    }

    /// Get next delay for retry
    pub fn get_retry_delay(&self, retry_count: u32) -> u64 {
        self.retry_policy.get_delay(retry_count)
    }

    /// Clear all items
    pub async fn clear(&self) {
        let mut items = self.items.write().await;
        items.clear();
    }
}

/// Queue statistics
#[derive(Debug, Clone, Default)]
pub struct QueueStats {
    pub total: usize,
    pub pending: usize,
    pub in_progress: usize,
    pub completed: usize,
    pub failed: usize,
    pub max_size: usize,
}

impl QueueStats {
    pub fn utilization(&self) -> f64 {
        if self.max_size == 0 {
            return 0.0;
        }
        self.total as f64 / self.max_size as f64
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::MemoryBackend;

    #[tokio::test]
    async fn test_enqueue_dequeue() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = OfflineQueue::new(storage, 100, RetryPolicy::default());

        let entry = DataEntry::new("test-key".to_string(), b"test-value".to_vec(), "device1".to_string());
        let id = queue.enqueue(entry.clone(), QueueOperation::Put(entry)).await.unwrap();

        assert!(!id.is_empty());

        let pending = queue.get_pending().await;
        assert_eq!(pending.len(), 1);
        assert_eq!(pending[0].id, id);
    }

    #[tokio::test]
    async fn test_queue_stats() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = OfflineQueue::new(storage, 100, RetryPolicy::default());

        let entry1 = DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string());
        let entry2 = DataEntry::new("key2".to_string(), b"value2".to_vec(), "device1".to_string());

        let _id1 = queue.enqueue(entry1.clone(), QueueOperation::Put(entry1)).await.unwrap();
        let _id2 = queue.enqueue(entry2.clone(), QueueOperation::Put(entry2)).await.unwrap();

        let stats = queue.stats().await;
        assert_eq!(stats.total, 2);
        assert_eq!(stats.pending, 2);
        assert_eq!(stats.failed, 0);
    }

    #[tokio::test]
    async fn test_purge_completed() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = OfflineQueue::new(storage, 100, RetryPolicy::default());

        let entry = DataEntry::new("key".to_string(), b"value".to_vec(), "device1".to_string());
        let id = queue.enqueue(entry.clone(), QueueOperation::Put(entry)).await.unwrap();

        queue.update_status(&id, QueueItemStatus::Completed).await.unwrap();
        
        let removed = queue.purge_completed().await.unwrap();
        assert_eq!(removed, 1);

        let stats = queue.stats().await;
        assert_eq!(stats.total, 0);
    }

    #[tokio::test]
    async fn test_retry_logic() {
        let storage = Arc::new(MemoryBackend::new());
        let queue = OfflineQueue::new(storage, 100, RetryPolicy::default());

        let entry = DataEntry::new("key".to_string(), b"value".to_vec(), "device1".to_string());
        let id = queue.enqueue(entry.clone(), QueueOperation::Put(entry)).await.unwrap();

        // Simulate failures
        for i in 1..=5 {
            queue.mark_failed(&id, format!("Error {}", i)).await.unwrap();
            let item = queue.get_by_id(&id).await.unwrap();
            
            if i < 5 {
                assert_eq!(item.status, QueueItemStatus::Pending);
            } else {
                assert_eq!(item.status, QueueItemStatus::Failed);
            }
        }
    }
}
