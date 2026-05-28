//! Storage backend implementations

use crate::{DataEntry, EdgeSyncError, Result};
use std::collections::HashMap;
use async_trait::async_trait;

/// Trait for storage backends
#[async_trait]
pub trait StorageBackend: Send + Sync {
    /// Get a data entry by key
    async fn get(&self, key: &str) -> Result<Option<DataEntry>>;
    
    /// Put a data entry
    async fn put(&self, entry: DataEntry) -> Result<()>;
    
    /// Delete a data entry by key
    async fn delete(&self, key: &str) -> Result<()>;
    
    /// List all keys with optional prefix
    async fn list_keys(&self, prefix: Option<&str>) -> Result<Vec<String>>;
    
    /// Get multiple entries by keys
    async fn get_batch(&self, keys: &[String]) -> Result<HashMap<String, DataEntry>>;
    
    /// Put multiple entries
    async fn put_batch(&self, entries: Vec<DataEntry>) -> Result<()>;
    
    /// Get all entries
    async fn get_all(&self) -> Result<HashMap<String, DataEntry>>;
    
    /// Compact the storage (if supported)
    async fn compact(&self) -> Result<()>;
    
    /// Get storage statistics
    async fn stats(&self) -> Result<StorageStats>;
}

/// Storage statistics
#[derive(Debug, Clone, Default)]
pub struct StorageStats {
    pub total_entries: u64,
    pub total_size_bytes: u64,
    pub deleted_entries: u64,
    pub pending_sync_count: u64,
}

/// RocksDB storage backend implementation
pub struct RocksDBBackend {
    db: rocksdb::DB,
}

impl RocksDBBackend {
    pub fn new(path: &str) -> Result<Self> {
        let mut opts = rocksdb::Options::default();
        opts.create_if_missing(true);
        opts.set_compression_type(rocksdb::DBCompressionType::Lz4);
        
        // Optimize for writes
        opts.set_write_buffer_size(64 * 1024 * 1024);
        opts.set_max_write_buffer_number(3);
        opts.set_target_file_size_base(64 * 1024 * 1024);
        
        let db = rocksdb::DB::open(&opts, path)?;
        
        Ok(Self { db })
    }
    
    fn serialize_entry(entry: &DataEntry) -> Result<Vec<u8>> {
        bincode::serialize(entry).map_err(|e| EdgeSyncError::SerializationError(e.to_string()))
    }
    
    fn deserialize_entry(bytes: &[u8]) -> Result<DataEntry> {
        bincode::deserialize(bytes).map_err(|e| EdgeSyncError::SerializationError(e.to_string()))
    }
}

#[async_trait]
impl StorageBackend for RocksDBBackend {
    async fn get(&self, key: &str) -> Result<Option<DataEntry>> {
        let bytes = self.db.get(key.as_bytes())?;
        match bytes {
            Some(data) => Ok(Some(Self::deserialize_entry(&data)?)),
            None => Ok(None),
        }
    }
    
    async fn put(&self, entry: DataEntry) -> Result<()> {
        let bytes = Self::serialize_entry(&entry)?;
        self.db.put(entry.key.as_bytes(), &bytes)?;
        Ok(())
    }
    
    async fn delete(&self, key: &str) -> Result<()> {
        self.db.delete(key.as_bytes())?;
        Ok(())
    }
    
    async fn list_keys(&self, prefix: Option<&str>) -> Result<Vec<String>> {
        let mut keys = Vec::new();
        let iter = self.db.iterator(rocksdb::IteratorMode::Start);
        
        for item in iter {
            match item {
                Ok((key, _)) => {
                    let key_str = String::from_utf8_lossy(&key).to_string();
                    if let Some(p) = prefix {
                        if key_str.starts_with(p) {
                            keys.push(key_str);
                        }
                    } else {
                        keys.push(key_str);
                    }
                }
                Err(e) => return Err(EdgeSyncError::StorageError(e.to_string())),
            }
        }
        
        Ok(keys)
    }
    
    async fn get_batch(&self, keys: &[String]) -> Result<HashMap<String, DataEntry>> {
        let mut result = HashMap::new();
        
        for key in keys {
            if let Some(entry) = self.get(key).await? {
                result.insert(key.clone(), entry);
            }
        }
        
        Ok(result)
    }
    
    async fn put_batch(&self, entries: Vec<DataEntry>) -> Result<()> {
        let mut batch = rocksdb::WriteBatch::default();
        
        for entry in entries {
            let bytes = Self::serialize_entry(&entry)?;
            batch.put(entry.key.as_bytes(), &bytes);
        }
        
        self.db.write(batch)?;
        Ok(())
    }
    
    async fn get_all(&self) -> Result<HashMap<String, DataEntry>> {
        let keys = self.list_keys(None).await?;
        self.get_batch(&keys).await
    }
    
    async fn compact(&self) -> Result<()> {
        self.db.compact_range(None::<&[u8]>, None::<&[u8]>);
        Ok(())
    }
    
    async fn stats(&self) -> Result<StorageStats> {
        let keys = self.list_keys(None).await?;
        let mut total_size = 0u64;
        let mut deleted_count = 0u64;
        
        for key in &keys {
            if let Some(entry) = self.get(key).await? {
                total_size += entry.value.len() as u64;
                if entry.deleted {
                    deleted_count += 1;
                }
            }
        }
        
        Ok(StorageStats {
            total_entries: keys.len() as u64,
            total_size_bytes: total_size,
            deleted_entries: deleted_count,
            pending_sync_count: 0, // Would need separate tracking
        })
    }
}

/// In-memory storage backend for testing
pub struct MemoryBackend {
    data: tokio::sync::RwLock<HashMap<String, DataEntry>>,
}

impl MemoryBackend {
    pub fn new() -> Self {
        Self {
            data: tokio::sync::RwLock::new(HashMap::new()),
        }
    }
}

impl Default for MemoryBackend {
    fn default() -> Self {
        Self::new()
    }
}

#[async_trait]
impl StorageBackend for MemoryBackend {
    async fn get(&self, key: &str) -> Result<Option<DataEntry>> {
        let data = self.data.read().await;
        Ok(data.get(key).cloned())
    }
    
    async fn put(&self, entry: DataEntry) -> Result<()> {
        let mut data = self.data.write().await;
        data.insert(entry.key.clone(), entry);
        Ok(())
    }
    
    async fn delete(&self, key: &str) -> Result<()> {
        let mut data = self.data.write().await;
        data.remove(key);
        Ok(())
    }
    
    async fn list_keys(&self, prefix: Option<&str>) -> Result<Vec<String>> {
        let data = self.data.read().await;
        let mut keys: Vec<String> = data.keys().cloned().collect();
        
        if let Some(p) = prefix {
            keys.retain(|k| k.starts_with(p));
        }
        
        Ok(keys)
    }
    
    async fn get_batch(&self, keys: &[String]) -> Result<HashMap<String, DataEntry>> {
        let data = self.data.read().await;
        let mut result = HashMap::new();
        
        for key in keys {
            if let Some(entry) = data.get(key) {
                result.insert(key.clone(), entry.clone());
            }
        }
        
        Ok(result)
    }
    
    async fn put_batch(&self, entries: Vec<DataEntry>) -> Result<()> {
        let mut data = self.data.write().await;
        for entry in entries {
            data.insert(entry.key.clone(), entry);
        }
        Ok(())
    }
    
    async fn get_all(&self) -> Result<HashMap<String, DataEntry>> {
        let data = self.data.read().await;
        Ok(data.clone())
    }
    
    async fn compact(&self) -> Result<()> {
        // No-op for memory backend
        Ok(())
    }
    
    async fn stats(&self) -> Result<StorageStats> {
        let data = self.data.read().await;
        let mut total_size = 0u64;
        let mut deleted_count = 0u64;
        
        for entry in data.values() {
            total_size += entry.value.len() as u64;
            if entry.deleted {
                deleted_count += 1;
            }
        }
        
        Ok(StorageStats {
            total_entries: data.len() as u64,
            total_size_bytes: total_size,
            deleted_entries: deleted_count,
            pending_sync_count: 0,
        })
    }
}
