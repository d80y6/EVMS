# Edge Sync Service Tests

use edge_sync::{Config, DataEntry, MemoryBackend, OfflineQueue, RetryPolicy, StorageBackend};
use std::sync::Arc;

#[tokio::test]
async fn test_basic_storage_operations() {
    let storage = Arc::new(MemoryBackend::new());
    
    let entry = DataEntry::new("test-key".to_string(), b"test-value".to_vec(), "device1".to_string());
    
    // Put entry
    storage.put(entry.clone()).await.unwrap();
    
    // Get entry
    let retrieved = storage.get("test-key").await.unwrap();
    assert!(retrieved.is_some());
    assert_eq!(retrieved.unwrap().value, b"test-value");
    
    // Delete entry
    storage.delete("test-key").await.unwrap();
    
    // Verify deletion
    let deleted = storage.get("test-key").await.unwrap();
    assert!(deleted.is_none());
}

#[tokio::test]
async fn test_batch_operations() {
    let storage = Arc::new(MemoryBackend::new());
    
    let entries = vec![
        DataEntry::new("key1".to_string(), b"value1".to_vec(), "device1".to_string()),
        DataEntry::new("key2".to_string(), b"value2".to_vec(), "device1".to_string()),
        DataEntry::new("key3".to_string(), b"value3".to_vec(), "device1".to_string()),
    ];
    
    // Batch put
    storage.put_batch(entries.clone()).await.unwrap();
    
    // List keys
    let keys = storage.list_keys(None).await.unwrap();
    assert_eq!(keys.len(), 3);
    
    // Batch get
    let retrieved = storage.get_batch(&keys).await.unwrap();
    assert_eq!(retrieved.len(), 3);
}

#[tokio::test]
async fn test_offline_queue_operations() {
    let storage = Arc::new(MemoryBackend::new());
    let queue = OfflineQueue::new(storage.clone(), 100, RetryPolicy::default());
    
    let entry = DataEntry::new("queue-key".to_string(), b"queue-value".to_vec(), "device1".to_string());
    
    // Enqueue
    let id = queue.enqueue(entry.clone(), edge_sync::offline_queue::QueueOperation::Put(entry)).await.unwrap();
    assert!(!id.is_empty());
    
    // Get pending
    let pending = queue.get_pending().await;
    assert_eq!(pending.len(), 1);
    
    // Get stats
    let stats = queue.stats().await;
    assert_eq!(stats.pending, 1);
    assert_eq!(stats.total, 1);
}

#[tokio::test]
async fn test_vector_clock_operations() {
    use edge_sync::VectorClock;
    
    let mut vc1 = VectorClock::new();
    vc1.increment("node1");
    vc1.increment("node1");
    
    let mut vc2 = VectorClock::new();
    vc2.increment("node2");
    vc2.increment("node2");
    vc2.increment("node2");
    
    // Test happens-before
    assert!(!vc1.happens_before(&vc2));
    assert!(!vc2.happens_before(&vc1));
    assert!(vc1.is_concurrent(&vc2));
    
    // Test merge
    vc1.merge(&vc2);
    assert_eq!(vc1.get("node1"), 2);
    assert_eq!(vc1.get("node2"), 3);
}

#[tokio::test]
async fn test_conflict_resolution() {
    use edge_sync::{ConflictResolutionStrategy, ConflictResolver};
    
    let resolver = ConflictResolver::new(ConflictResolutionStrategy::LastWriteWins);
    
    let local = DataEntry {
        key: "conflict-key".to_string(),
        value: b"local-value".to_vec(),
        content_type: "text/plain".to_string(),
        created_at: 100,
        updated_at: 100,
        metadata: std::collections::HashMap::new(),
        deleted: false,
        version: 1,
        device_id: "device1".to_string(),
    };
    
    let remote = DataEntry {
        key: "conflict-key".to_string(),
        value: b"remote-value".to_vec(),
        content_type: "text/plain".to_string(),
        created_at: 100,
        updated_at: 200, // Later timestamp
        metadata: std::collections::HashMap::new(),
        deleted: false,
        version: 1,
        device_id: "device2".to_string(),
    };
    
    let conflict = resolver.create_conflict(&local, &remote, None);
    let result = resolver.resolve(&conflict).unwrap();
    
    // Remote should win due to later timestamp
    assert_eq!(result.resolved_entry.value, b"remote-value");
}

#[tokio::test]
async fn test_sync_engine_offline_mode() {
    use edge_sync::{ConflictResolver, ConflictResolutionStrategy, SyncEngine};
    
    let storage = Arc::new(MemoryBackend::new());
    let queue = Arc::new(OfflineQueue::new(storage.clone(), 100, RetryPolicy::default()));
    let resolver = Arc::new(ConflictResolver::new(ConflictResolutionStrategy::LastWriteWins));
    
    let engine = SyncEngine::new(storage.clone(), queue.clone(), resolver.clone(), "device1".to_string());
    
    // Start offline
    engine.set_online(false).await;
    
    let entry = DataEntry::new("sync-key".to_string(), b"sync-value".to_vec(), "device1".to_string());
    engine.put(entry.clone()).await.unwrap();
    
    // Entry should be queued, not in storage
    assert!(engine.get("sync-key").await.unwrap().is_none());
    
    let pending = queue.get_pending().await;
    assert_eq!(pending.len(), 1);
    
    // Go online and process
    engine.set_online(true).await;
    let result = engine.process_queue().await.unwrap();
    
    assert_eq!(result.success_count, 1);
    
    // Now entry should be in storage
    assert!(engine.get("sync-key").await.unwrap().is_some());
}

#[tokio::test]
async fn test_crdt_g_counter() {
    use edge_sync::crdt::GCounter;
    
    let mut counter = GCounter::new();
    counter.increment("node1", 5);
    counter.increment("node2", 3);
    
    assert_eq!(counter.value(), 8);
    
    let mut counter2 = GCounter::new();
    counter2.increment("node1", 2);
    counter2.increment("node3", 7);
    
    counter.merge(&counter2);
    
    // Should take max of each node's count
    assert_eq!(counter.value(), 17); // max(5,2) + 3 + 7 = 17
}

#[tokio::test]
async fn test_crdt_or_set() {
    use edge_sync::crdt::ORSet;
    
    let mut set = ORSet::new();
    
    set.add("a", "device1".to_string(), 1);
    set.add("b", "device1".to_string(), 2);
    set.add("c", "device1".to_string(), 3);
    
    assert!(set.contains(&"a"));
    assert!(set.contains(&"b"));
    assert!(set.contains(&"c"));
    
    set.remove(&"b", "device1".to_string(), 4);
    
    assert!(set.contains(&"a"));
    assert!(!set.contains(&"b"));
    assert!(set.contains(&"c"));
}

#[tokio::test]
async fn test_replication_manager() {
    use edge_sync::replication::ReplicationManager;
    
    let manager = ReplicationManager::new("device1".to_string(), 1000);
    
    // Register peers
    manager.register_peer("device2".to_string(), "http://device2:8080".to_string()).await;
    manager.register_peer("device3".to_string(), "http://device3:8080".to_string()).await;
    
    let peers = manager.get_peers().await;
    assert_eq!(peers.len(), 2);
    
    // Record replication
    let entry = DataEntry::new("rep-key".to_string(), b"rep-value".to_vec(), "device1".to_string());
    manager.record_replication("put", &entry, "device2", true, None).await;
    
    let stats = manager.get_stats().await;
    assert_eq!(stats.total_operations, 1);
    assert_eq!(stats.peer_count, 2);
}
