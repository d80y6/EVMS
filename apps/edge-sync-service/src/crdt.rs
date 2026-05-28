//! CRDT (Conflict-free Replicated Data Types) implementations

use crate::{DataEntry, VectorClock};
use std::collections::{HashMap, BTreeMap};
use serde::{Deserialize, Serialize};

/// Last-Writer-Wins Register CRDT
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LWWRegister<T> {
    value: Option<T>,
    timestamp: u64,
    device_id: String,
}

impl<T: Clone> LWWRegister<T> {
    pub fn new(value: T, timestamp: u64, device_id: String) -> Self {
        Self {
            value: Some(value),
            timestamp,
            device_id,
        }
    }

    pub fn get(&self) -> Option<&T> {
        self.value.as_ref()
    }

    pub fn set(&mut self, value: T, timestamp: u64, device_id: String) {
        if timestamp > self.timestamp || (timestamp == self.timestamp && device_id > self.device_id) {
            self.value = Some(value);
            self.timestamp = timestamp;
            self.device_id = device_id;
        }
    }

    pub fn merge(&mut self, other: &Self) {
        if other.timestamp > self.timestamp 
            || (other.timestamp == self.timestamp && other.device_id > self.device_id) {
            self.value = other.value.clone();
            self.timestamp = other.timestamp;
            self.device_id = other.device_id.clone();
        }
    }

    pub fn with_value(&self, value: T) -> Self {
        Self {
            value: Some(value),
            timestamp: self.timestamp,
            device_id: self.device_id.clone(),
        }
    }
}

/// G-Counter (Grow-only Counter) CRDT
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct GCounter {
    counts: HashMap<String, u64>,
}

impl GCounter {
    pub fn new() -> Self {
        Self {
            counts: HashMap::new(),
        }
    }

    pub fn increment(&mut self, node_id: &str, amount: u64) {
        let counter = self.counts.entry(node_id.to_string()).or_insert(0);
        *counter += amount;
    }

    pub fn value(&self) -> u64 {
        self.counts.values().sum()
    }

    pub fn merge(&mut self, other: &GCounter) {
        for (node_id, &count) in &other.counts {
            let entry = self.counts.entry(node_id.clone()).or_insert(0);
            *entry = (*entry).max(count);
        }
    }
}

/// PN-Counter (Positive-Negative Counter) CRDT
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct PNCounter {
    positive: GCounter,
    negative: GCounter,
}

impl PNCounter {
    pub fn new() -> Self {
        Self {
            positive: GCounter::new(),
            negative: GCounter::new(),
        }
    }

    pub fn increment(&mut self, node_id: &str, amount: u64) {
        self.positive.increment(node_id, amount);
    }

    pub fn decrement(&mut self, node_id: &str, amount: u64) {
        self.negative.increment(node_id, amount);
    }

    pub fn value(&self) -> i64 {
        self.positive.value() as i64 - self.negative.value() as i64
    }

    pub fn merge(&mut self, other: &PNCounter) {
        self.positive.merge(&other.positive);
        self.negative.merge(&other.negative);
    }
}

/// OR-Set (Observed-Remove Set) CRDT
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ORSet<T> 
where
    T: Eq + std::hash::Hash + Clone + std::fmt::Debug,
{
    elements: HashMap<T, Vec<(String, u64)>>, // element -> [(device_id, timestamp)]
    tombstones: HashMap<T, Vec<(String, u64)>>,
}

impl<T> Default for ORSet<T>
where
    T: Eq + std::hash::Hash + Clone + std::fmt::Debug,
{
    fn default() -> Self {
        Self::new()
    }
}

impl<T> ORSet<T>
where
    T: Eq + std::hash::Hash + Clone + std::fmt::Debug,
{
    pub fn new() -> Self {
        Self {
            elements: HashMap::new(),
            tombstones: HashMap::new(),
        }
    }

    pub fn add(&mut self, element: T, device_id: String, timestamp: u64) {
        let entries = self.elements.entry(element).or_insert_with(Vec::new);
        entries.push((device_id, timestamp));
    }

    pub fn remove(&mut self, element: &T, device_id: String, timestamp: u64) {
        if let Some(entries) = self.elements.get(element) {
            let tombstone_entries = self.tombstones.entry(element.clone()).or_insert_with(Vec::new);
            for (dev, ts) in entries {
                if *ts <= timestamp {
                    tombstone_entries.push((dev.clone(), *ts));
                }
            }
        }
    }

    pub fn contains(&self, element: &T) -> bool {
        if let Some(entries) = self.elements.get(element) {
            if let Some(tombstone_entries) = self.tombstones.get(element) {
                // Element exists if there's at least one add that wasn't tombstoned
                return entries.iter().any(|(dev, ts)| {
                    !tombstone_entries.iter().any(|(t_dev, t_ts)| {
                        t_dev == dev && t_ts >= ts
                    })
                });
            }
            return !entries.is_empty();
        }
        false
    }

    pub fn elements(&self) -> Vec<&T> {
        self.elements.keys().filter(|e| self.contains(e)).collect()
    }

    pub fn merge(&mut self, other: &Self) {
        // Merge elements
        for (element, entries) in &other.elements {
            let my_entries = self.elements.entry(element.clone()).or_insert_with(Vec::new);
            for entry in entries {
                if !my_entries.contains(entry) {
                    my_entries.push(entry.clone());
                }
            }
        }

        // Merge tombstones
        for (element, entries) in &other.tombstones {
            let my_tombstones = self.tombstones.entry(element.clone()).or_insert_with(Vec::new);
            for entry in entries {
                if !my_tombstones.contains(entry) {
                    my_tombstones.push(entry.clone());
                }
            }
        }
    }
}

/// LWW-Map (Last-Writer-Wins Map) CRDT
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LWWMap<K, V>
where
    K: Eq + std::hash::Hash + Clone + std::fmt::Debug,
    V: Clone,
{
    data: HashMap<K, LWWRegister<V>>,
}

impl<K, V> Default for LWWMap<K, V>
where
    K: Eq + std::hash::Hash + Clone + std::fmt::Debug,
    V: Clone,
{
    fn default() -> Self {
        Self::new()
    }
}

impl<K, V> LWWMap<K, V>
where
    K: Eq + std::hash::Hash + Clone + std::fmt::Debug,
    V: Clone,
{
    pub fn new() -> Self {
        Self {
            data: HashMap::new(),
        }
    }

    pub fn put(&mut self, key: K, value: V, timestamp: u64, device_id: String) {
        let register = self.data.entry(key).or_insert_with(|| {
            LWWRegister::new(value.clone(), 0, String::new())
        });
        register.set(value, timestamp, device_id);
    }

    pub fn get(&self, key: &K) -> Option<&V> {
        self.data.get(key).and_then(|r| r.get())
    }

    pub fn remove(&mut self, key: &K, timestamp: u64, device_id: String) {
        if let Some(register) = self.data.get_mut(key) {
            // Set to a "tombstone" value with high timestamp
            register.set(unsafe { std::mem::zeroed() }, timestamp, device_id);
        }
    }

    pub fn merge(&mut self, other: &Self) {
        for (key, other_register) in &other.data {
            if let Some(my_register) = self.data.get_mut(key) {
                my_register.merge(other_register);
            } else {
                self.data.insert(key.clone(), other_register.clone());
            }
        }
    }

    pub fn keys(&self) -> impl Iterator<Item = &K> {
        self.data.keys()
    }

    pub fn iter(&self) -> impl Iterator<Item = (&K, &LWWRegister<V>)> {
        self.data.iter()
    }
}

/// Utility functions for merging DataEntries using CRDTs
pub fn merge_data_entries(local: &DataEntry, remote: &DataEntry) -> DataEntry {
    // Use LWW based on updated_at timestamp
    if remote.updated_at > local.updated_at {
        remote.clone()
    } else if local.updated_at > remote.updated_at {
        local.clone()
    } else {
        // Same timestamp, use device_id as tiebreaker
        if remote.device_id > local.device_id {
            remote.clone()
        } else {
            local.clone()
        }
    }
}

/// Merge vector clocks and detect conflicts
pub fn detect_conflict(local_clock: &VectorClock, remote_clock: &VectorClock) -> bool {
    local_clock.is_concurrent(remote_clock)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_lww_register() {
        let mut reg = LWWRegister::new(42, 100, "device1".to_string());
        assert_eq!(reg.get(), Some(&42));

        reg.set(100, 200, "device2".to_string());
        assert_eq!(reg.get(), Some(&100));

        // Older write should be ignored
        reg.set(50, 150, "device3".to_string());
        assert_eq!(reg.get(), Some(&100));
    }

    #[test]
    fn test_g_counter() {
        let mut counter = GCounter::new();
        counter.increment("node1", 5);
        counter.increment("node2", 3);
        
        assert_eq!(counter.value(), 8);

        let mut counter2 = GCounter::new();
        counter2.increment("node1", 3);
        counter2.increment("node3", 7);

        counter.merge(&counter2);
        assert_eq!(counter.value(), 15); // max(5,3) + 3 + 7 = 15
    }

    #[test]
    fn test_pn_counter() {
        let mut counter = PNCounter::new();
        counter.increment("node1", 10);
        counter.decrement("node1", 3);
        
        assert_eq!(counter.value(), 7);
    }

    #[test]
    fn test_or_set() {
        let mut set = ORSet::new();
        set.add("a", "device1".to_string(), 1);
        set.add("b", "device1".to_string(), 2);
        
        assert!(set.contains(&"a"));
        assert!(set.contains(&"b"));

        set.remove(&"a", "device1".to_string(), 3);
        assert!(!set.contains(&"a"));
        assert!(set.contains(&"b"));
    }

    #[test]
    fn test_lww_map() {
        let mut map = LWWMap::new();
        map.put("key1", "value1", 100, "device1".to_string());
        map.put("key2", "value2", 200, "device1".to_string());

        assert_eq!(map.get(&"key1"), Some(&"value1"));
        assert_eq!(map.get(&"key2"), Some(&"value2"));

        let mut map2 = LWWMap::new();
        map2.put("key1", "updated_value1", 300, "device2".to_string());

        map.merge(&map2);
        assert_eq!(map.get(&"key1"), Some(&"updated_value1"));
    }
}
