//! Conflict resolver for handling data conflicts during synchronization

use crate::{Conflict, ConflictResolutionStrategy, DataEntry, EdgeSyncError, Result};
use crate::crdt::merge_data_entries;
use std::sync::Arc;
use parking_lot::RwLock;

/// Conflict resolution result
#[derive(Debug, Clone)]
pub struct ResolutionResult {
    pub resolved_entry: DataEntry,
    pub strategy_used: ConflictResolutionStrategy,
    pub was_merged: bool,
}

/// Conflict resolver component
pub struct ConflictResolver {
    default_strategy: ConflictResolutionStrategy,
    custom_resolvers: Arc<RwLock<std::collections::HashMap<String, Box<dyn CustomResolver + Send + Sync>>>>,
}

impl ConflictResolver {
    pub fn new(default_strategy: ConflictResolutionStrategy) -> Self {
        Self {
            default_strategy,
            custom_resolvers: Arc::new(RwLock::new(std::collections::HashMap::new())),
        }
    }

    /// Register a custom resolver for a specific content type
    pub fn register_resolver(
        &self,
        content_type: &str,
        resolver: Box<dyn CustomResolver + Send + Sync>,
    ) {
        let mut resolvers = self.custom_resolvers.write();
        resolvers.insert(content_type.to_string(), resolver);
    }

    /// Resolve a conflict between local and remote entries
    pub fn resolve(&self, conflict: &Conflict) -> Result<ResolutionResult> {
        match conflict.strategy {
            ConflictResolutionStrategy::LastWriteWins => {
                self.resolve_lww(&conflict.local_entry, &conflict.remote_entry)
            }
            ConflictResolutionStrategy::Merge => {
                self.resolve_merge(&conflict.local_entry, &conflict.remote_entry)
            }
            ConflictResolutionStrategy::Manual => {
                Err(EdgeSyncError::ConflictError(
                    "Manual resolution required".to_string()
                ))
            }
            ConflictResolutionStrategy::Custom => {
                self.resolve_custom(&conflict.local_entry, &conflict.remote_entry)
            }
        }
    }

    /// Last-Writer-Wins resolution
    fn resolve_lww(&self, local: &DataEntry, remote: &DataEntry) -> Result<ResolutionResult> {
        let resolved = merge_data_entries(local, remote);
        
        Ok(ResolutionResult {
            resolved_entry: resolved,
            strategy_used: ConflictResolutionStrategy::LastWriteWins,
            was_merged: false,
        })
    }

    /// Merge-based resolution
    fn resolve_merge(&self, local: &DataEntry, remote: &DataEntry) -> Result<ResolutionResult> {
        // Try to merge metadata
        let mut merged_metadata = local.metadata.clone();
        for (key, value) in &remote.metadata {
            if !merged_metadata.contains_key(key) {
                merged_metadata.insert(key.clone(), value.clone());
            }
        }

        // Use LWW for the actual value but preserve merged metadata
        let base_resolved = merge_data_entries(local, remote);
        let mut resolved = base_resolved;
        resolved.metadata = merged_metadata;

        Ok(ResolutionResult {
            resolved_entry: resolved,
            strategy_used: ConflictResolutionStrategy::Merge,
            was_merged: true,
        })
    }

    /// Custom resolution using registered resolvers
    fn resolve_custom(&self, local: &DataEntry, remote: &DataEntry) -> Result<ResolutionResult> {
        let resolvers = self.custom_resolvers.read();
        
        if let Some(resolver) = resolvers.get(&local.content_type) {
            match resolver.resolve(local, remote) {
                Ok(entry) => Ok(ResolutionResult {
                    resolved_entry: entry,
                    strategy_used: ConflictResolutionStrategy::Custom,
                    was_merged: true,
                }),
                Err(e) => Err(e),
            }
        } else {
            // Fall back to LWW if no custom resolver found
            self.resolve_lww(local, remote)
        }
    }

    /// Detect conflicts between entries using vector clocks
    pub fn detect_conflict(&self, local: &DataEntry, remote: &DataEntry) -> bool {
        // Simple timestamp-based conflict detection
        // In a full implementation, this would use vector clocks
        let time_threshold = 1000; // 1 second in milliseconds
        
        i64::abs(local.updated_at as i64 - remote.updated_at as i64) < time_threshold
            && local.device_id != remote.device_id
            && local.value != remote.value
    }

    /// Create a conflict object from two conflicting entries
    pub fn create_conflict(
        &self,
        local: &DataEntry,
        remote: &DataEntry,
        strategy: Option<ConflictResolutionStrategy>,
    ) -> Conflict {
        use std::time::{SystemTime, UNIX_EPOCH};
        
        Conflict {
            id: uuid::Uuid::new_v4().to_string(),
            key: local.key.clone(),
            local_entry: local.clone(),
            remote_entry: remote.clone(),
            strategy: strategy.unwrap_or(self.default_strategy),
            created_at: SystemTime::now()
                .duration_since(UNIX_EPOCH)
                .unwrap()
                .as_millis() as u64,
        }
    }

    /// Batch resolve multiple conflicts
    pub fn resolve_batch(&self, conflicts: &[Conflict]) -> Vec<Result<ResolutionResult>> {
        conflicts.iter().map(|c| self.resolve(c)).collect()
    }
}

/// Trait for custom conflict resolvers
pub trait CustomResolver {
    fn resolve(&self, local: &DataEntry, remote: &DataEntry) -> Result<DataEntry>;
}

/// JSON merge resolver for structured data
pub struct JsonMergeResolver;

impl CustomResolver for JsonMergeResolver {
    fn resolve(&self, local: &DataEntry, remote: &DataEntry) -> Result<DataEntry> {
        let local_json: serde_json::Value = serde_json::from_slice(&local.value)
            .map_err(|e| EdgeSyncError::SerializationError(e.to_string()))?;
        
        let remote_json: serde_json::Value = serde_json::from_slice(&remote.value)
            .map_err(|e| EdgeSyncError::SerializationError(e.to_string()))?;

        let merged = merge_json_values(local_json, remote_json);
        
        let merged_bytes = serde_json::to_vec(&merged)
            .map_err(|e| EdgeSyncError::SerializationError(e.to_string()))?;

        let mut result = local.clone();
        result.value = merged_bytes;
        result.updated_at = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        
        Ok(result)
    }
}

/// Deep merge two JSON values
fn merge_json_values(local: serde_json::Value, remote: serde_json::Value) -> serde_json::Value {
    match (local, remote) {
        (serde_json::Value::Object(mut local_map), serde_json::Value::Object(remote_map)) => {
            for (key, remote_value) in remote_map {
                if let Some(local_value) = local_map.remove(&key) {
                    local_map.insert(key, merge_json_values(local_value, remote_value));
                } else {
                    local_map.insert(key, remote_value);
                }
            }
            serde_json::Value::Object(local_map)
        }
        (_, remote) => remote, // Default to remote for non-object types
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn create_test_entry(key: &str, value: &[u8], timestamp: u64, device_id: &str) -> DataEntry {
        DataEntry {
            key: key.to_string(),
            value: value.to_vec(),
            content_type: "application/octet-stream".to_string(),
            created_at: timestamp,
            updated_at: timestamp,
            metadata: std::collections::HashMap::new(),
            deleted: false,
            version: 1,
            device_id: device_id.to_string(),
        }
    }

    #[test]
    fn test_lww_resolution() {
        let resolver = ConflictResolver::new(ConflictResolutionStrategy::LastWriteWins);
        
        let local = create_test_entry("key1", b"value1", 100, "device1");
        let remote = create_test_entry("key1", b"value2", 200, "device2");
        
        let conflict = resolver.create_conflict(&local, &remote, None);
        let result = resolver.resolve(&conflict).unwrap();
        
        assert_eq!(result.resolved_entry.value, b"value2");
        assert_eq!(result.strategy_used, ConflictResolutionStrategy::LastWriteWins);
    }

    #[test]
    fn test_merge_resolution() {
        let resolver = ConflictResolver::new(ConflictResolutionStrategy::Merge);
        
        let mut local_meta = std::collections::HashMap::new();
        local_meta.insert("local_key".to_string(), "local_value".to_string());
        
        let mut remote_meta = std::collections::HashMap::new();
        remote_meta.insert("remote_key".to_string(), "remote_value".to_string());
        
        let mut local = create_test_entry("key1", b"value1", 100, "device1");
        local.metadata = local_meta;
        
        let mut remote = create_test_entry("key1", b"value2", 200, "device2");
        remote.metadata = remote_meta;
        
        let conflict = resolver.create_conflict(&local, &remote, None);
        let result = resolver.resolve(&conflict).unwrap();
        
        // Should have both metadata keys
        assert!(result.resolved_entry.metadata.contains_key("local_key"));
        assert!(result.resolved_entry.metadata.contains_key("remote_key"));
        assert!(result.was_merged);
    }

    #[test]
    fn test_json_merge_resolver() {
        let resolver = ConflictResolver::new(ConflictResolutionStrategy::Custom);
        resolver.register_resolver("application/json", Box::new(JsonMergeResolver));
        
        let local_json = br#"{"a": 1, "b": 2}"#;
        let remote_json = br#"{"b": 3, "c": 4}"#;
        
        let local = create_test_entry("key1", local_json, 100, "device1");
        let remote = create_test_entry("key1", remote_json, 200, "device2");
        
        let conflict = resolver.create_conflict(&local, &remote, Some(ConflictResolutionStrategy::Custom));
        let result = resolver.resolve(&conflict).unwrap();
        
        let merged: serde_json::Value = serde_json::from_slice(&result.resolved_entry.value).unwrap();
        assert_eq!(merged["a"], 1);
        assert_eq!(merged["b"], 3); // Remote wins for conflicting key
        assert_eq!(merged["c"], 4);
    }
}
