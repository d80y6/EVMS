//! Vector clock implementation for causality tracking

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use crate::{EdgeSyncError, Result};

/// Vector clock for tracking causality in distributed systems
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub struct VectorClock {
    clocks: HashMap<String, u64>,
}

impl VectorClock {
    /// Create a new empty vector clock
    pub fn new() -> Self {
        Self {
            clocks: HashMap::new(),
        }
    }

    /// Create a vector clock from a HashMap
    pub fn from_map(clocks: HashMap<String, u64>) -> Self {
        Self { clocks }
    }

    /// Increment the clock for a specific node
    pub fn increment(&mut self, node_id: &str) {
        let counter = self.clocks.entry(node_id.to_string()).or_insert(0);
        *counter += 1;
    }

    /// Get the clock value for a specific node
    pub fn get(&self, node_id: &str) -> u64 {
        *self.clocks.get(node_id).unwrap_or(&0)
    }

    /// Set the clock value for a specific node
    pub fn set(&mut self, node_id: &str, value: u64) {
        self.clocks.insert(node_id.to_string(), value);
    }

    /// Merge two vector clocks by taking the maximum of each component
    pub fn merge(&mut self, other: &VectorClock) {
        for (node_id, &value) in &other.clocks {
            let counter = self.clocks.entry(node_id.clone()).or_insert(0);
            *counter = (*counter).max(value);
        }
    }

    /// Check if this vector clock happens-before another
    /// Returns true if self < other (strictly)
    pub fn happens_before(&self, other: &VectorClock) -> bool {
        let mut strictly_less = false;
        
        // Collect all node IDs
        let all_nodes: std::collections::HashSet<_> = self.clocks.keys()
            .chain(other.clocks.keys())
            .collect();
        
        for node_id in all_nodes {
            let self_val = self.get(node_id);
            let other_val = other.get(node_id);
            
            if self_val > other_val {
                return false;
            }
            if self_val < other_val {
                strictly_less = true;
            }
        }
        
        strictly_less
    }

    /// Check if two vector clocks are concurrent
    pub fn is_concurrent(&self, other: &VectorClock) -> bool {
        !self.happens_before(other) && !other.happens_before(self) && self != other
    }

    /// Get all node IDs in this vector clock
    pub fn nodes(&self) -> Vec<&String> {
        self.clocks.keys().collect()
    }

    /// Get the total number of events across all nodes
    pub fn total_events(&self) -> u64 {
        self.clocks.values().sum()
    }

    /// Convert to HashMap
    pub fn to_map(&self) -> HashMap<String, u64> {
        self.clocks.clone()
    }

    /// Serialize to bytes
    pub fn to_bytes(&self) -> Result<Vec<u8>> {
        bincode::serialize(self).map_err(|e| EdgeSyncError::SerializationError(e.to_string()))
    }

    /// Deserialize from bytes
    pub fn from_bytes(bytes: &[u8]) -> Result<Self> {
        bincode::deserialize(bytes).map_err(|e| EdgeSyncError::SerializationError(e.to_string()))
    }
}

impl Default for VectorClock {
    fn default() -> Self {
        Self::new()
    }
}

impl std::fmt::Display for VectorClock {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        let pairs: Vec<_> = self.clocks.iter()
            .map(|(k, v)| format!("{}:{}", k, v))
            .collect();
        write!(f, "[{}]", pairs.join(", "))
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_increment() {
        let mut vc = VectorClock::new();
        vc.increment("node1");
        assert_eq!(vc.get("node1"), 1);
        
        vc.increment("node1");
        assert_eq!(vc.get("node1"), 2);
        
        vc.increment("node2");
        assert_eq!(vc.get("node2"), 1);
    }

    #[test]
    fn test_merge() {
        let mut vc1 = VectorClock::new();
        vc1.increment("node1");
        vc1.increment("node1");
        
        let mut vc2 = VectorClock::new();
        vc2.increment("node2");
        vc2.increment("node2");
        vc2.increment("node2");
        
        vc1.merge(&vc2);
        
        assert_eq!(vc1.get("node1"), 2);
        assert_eq!(vc1.get("node2"), 3);
    }

    #[test]
    fn test_happens_before() {
        let mut vc1 = VectorClock::new();
        vc1.increment("node1");
        
        let mut vc2 = VectorClock::new();
        vc2.increment("node1");
        vc2.increment("node1");
        
        assert!(vc1.happens_before(&vc2));
        assert!(!vc2.happens_before(&vc1));
    }

    #[test]
    fn test_concurrent() {
        let mut vc1 = VectorClock::new();
        vc1.increment("node1");
        
        let mut vc2 = VectorClock::new();
        vc2.increment("node2");
        
        assert!(vc1.is_concurrent(&vc2));
        assert!(vc2.is_concurrent(&vc1));
    }

    #[test]
    fn test_serialization() {
        let mut vc = VectorClock::new();
        vc.increment("node1");
        vc.increment("node2");
        
        let bytes = vc.to_bytes().unwrap();
        let restored = VectorClock::from_bytes(&bytes).unwrap();
        
        assert_eq!(vc, restored);
    }
}
