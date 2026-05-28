use std::collections::HashMap;
use dashmap::DashMap;

pub struct VectorCache {
    max_size: usize,
    entries: DashMap<String, CachedEntry>,
}

struct CachedEntry {
    vector: Vec<f32>,
    accessed_at: std::time::Instant,
}

impl VectorCache {
    pub fn new(max_size: usize) -> Self {
        Self {
            max_size,
            entries: DashMap::new(),
        }
    }

    pub fn get(&self, key: &str) -> Option<Vec<f32>> {
        self.entries.get(key).map(|e| {
            e.accessed_at = std::time::Instant::now();
            e.vector.clone()
        })
    }

    pub fn insert(&self, key: String, vector: Vec<f32>) {
        if self.entries.len() >= self.max_size {
            self.evict_one();
        }
        self.entries.insert(key, CachedEntry {
            vector,
            accessed_at: std::time::Instant::now(),
        });
    }

    fn evict_one(&self) {
        let mut oldest_key = None;
        let mut oldest_time = std::time::Instant::now();
        
        for entry in self.entries.iter() {
            if entry.value().accessed_at < oldest_time {
                oldest_time = entry.value().accessed_at;
                oldest_key = Some(entry.key().clone());
            }
        }
        
        if let Some(key) = oldest_key {
            self.entries.remove(&key);
        }
    }

    pub fn len(&self) -> usize {
        self.entries.len()
    }

    pub fn clear(&self) {
        self.entries.clear();
    }
}
