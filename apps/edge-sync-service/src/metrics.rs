//! Metrics collection for edge sync service

use metrics::{counter, gauge, histogram};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::Instant;

/// Metrics recorder for edge sync operations
pub struct SyncMetrics {
    sync_operations_total: AtomicU64,
    sync_success_total: AtomicU64,
    sync_failure_total: AtomicU64,
    conflicts_detected_total: AtomicU64,
    conflicts_resolved_total: AtomicU64,
    queue_size_current: AtomicU64,
    bytes_synced_total: AtomicU64,
    entries_synced_total: AtomicU64,
    offline_events_total: AtomicU64,
    online_events_total: AtomicU64,
}

impl Default for SyncMetrics {
    fn default() -> Self {
        Self::new()
    }
}

impl SyncMetrics {
    pub fn new() -> Self {
        Self {
            sync_operations_total: AtomicU64::new(0),
            sync_success_total: AtomicU64::new(0),
            sync_failure_total: AtomicU64::new(0),
            conflicts_detected_total: AtomicU64::new(0),
            conflicts_resolved_total: AtomicU64::new(0),
            queue_size_current: AtomicU64::new(0),
            bytes_synced_total: AtomicU64::new(0),
            entries_synced_total: AtomicU64::new(0),
            offline_events_total: AtomicU64::new(0),
            online_events_total: AtomicU64::new(0),
        }
    }

    /// Record a sync operation start
    pub fn record_sync_start(&self) -> SyncOperationTimer {
        self.sync_operations_total.fetch_add(1, Ordering::Relaxed);
        SyncOperationTimer::new(Instant::now())
    }

    /// Record a successful sync
    pub fn record_sync_success(&self, bytes: u64, entries: u64) {
        self.sync_success_total.fetch_add(1, Ordering::Relaxed);
        self.bytes_synced_total.fetch_add(bytes, Ordering::Relaxed);
        self.entries_synced_total.fetch_add(entries, Ordering::Relaxed);
        
        counter!("edge_sync_success_total").increment(1);
        histogram!("edge_sync_bytes_total").record(bytes as f64);
    }

    /// Record a failed sync
    pub fn record_sync_failure(&self, error_type: &str) {
        self.sync_failure_total.fetch_add(1, Ordering::Relaxed);
        counter!("edge_sync_failure_total", "error_type" => error_type.to_string()).increment(1);
    }

    /// Record a detected conflict
    pub fn record_conflict_detected(&self) {
        self.conflicts_detected_total.fetch_add(1, Ordering::Relaxed);
        counter!("edge_sync_conflicts_detected_total").increment(1);
    }

    /// Record a resolved conflict
    pub fn record_conflict_resolved(&self, strategy: &str) {
        self.conflicts_resolved_total.fetch_add(1, Ordering::Relaxed);
        counter!("edge_sync_conflicts_resolved_total", "strategy" => strategy.to_string()).increment(1);
    }

    /// Update current queue size
    pub fn update_queue_size(&self, size: u64) {
        self.queue_size_current.store(size, Ordering::Relaxed);
        gauge!("edge_sync_queue_size_current").set(size as f64);
    }

    /// Record an offline event
    pub fn record_offline_event(&self) {
        self.offline_events_total.fetch_add(1, Ordering::Relaxed);
        counter!("edge_sync_offline_events_total").increment(1);
    }

    /// Record an online event
    pub fn record_online_event(&self) {
        self.online_events_total.fetch_add(1, Ordering::Relaxed);
        counter!("edge_sync_online_events_total").increment(1);
    }

    /// Get current metrics snapshot
    pub fn snapshot(&self) -> MetricsSnapshot {
        MetricsSnapshot {
            sync_operations_total: self.sync_operations_total.load(Ordering::Relaxed),
            sync_success_total: self.sync_success_total.load(Ordering::Relaxed),
            sync_failure_total: self.sync_failure_total.load(Ordering::Relaxed),
            conflicts_detected_total: self.conflicts_detected_total.load(Ordering::Relaxed),
            conflicts_resolved_total: self.conflicts_resolved_total.load(Ordering::Relaxed),
            queue_size_current: self.queue_size_current.load(Ordering::Relaxed),
            bytes_synced_total: self.bytes_synced_total.load(Ordering::Relaxed),
            entries_synced_total: self.entries_synced_total.load(Ordering::Relaxed),
            offline_events_total: self.offline_events_total.load(Ordering::Relaxed),
            online_events_total: self.online_events_total.load(Ordering::Relaxed),
        }
    }

    /// Export metrics in Prometheus format
    pub fn export_prometheus(&self) -> String {
        let snapshot = self.snapshot();
        format!(
            r#"# HELP edge_sync_operations_total Total number of sync operations
# TYPE edge_sync_operations_total counter
edge_sync_operations_total {}

# HELP edge_sync_success_total Total number of successful syncs
# TYPE edge_sync_success_total counter
edge_sync_success_total {}

# HELP edge_sync_failure_total Total number of failed syncs
# TYPE edge_sync_failure_total counter
edge_sync_failure_total {}

# HELP edge_sync_conflicts_detected_total Total conflicts detected
# TYPE edge_sync_conflicts_detected_total counter
edge_sync_conflicts_detected_total {}

# HELP edge_sync_conflicts_resolved_total Total conflicts resolved
# TYPE edge_sync_conflicts_resolved_total counter
edge_sync_conflicts_resolved_total {}

# HELP edge_sync_queue_size_current Current queue size
# TYPE edge_sync_queue_size_current gauge
edge_sync_queue_size_current {}

# HELP edge_sync_bytes_total Total bytes synced
# TYPE edge_sync_bytes_total counter
edge_sync_bytes_total {}

# HELP edge_sync_entries_total Total entries synced
# TYPE edge_sync_entries_total counter
edge_sync_entries_total {}

# HELP edge_sync_offline_events_total Total offline events
# TYPE edge_sync_offline_events_total counter
edge_sync_offline_events_total {}

# HELP edge_sync_online_events_total Total online events
# TYPE edge_sync_online_events_total counter
edge_sync_online_events_total {}
"#,
            snapshot.sync_operations_total,
            snapshot.sync_success_total,
            snapshot.sync_failure_total,
            snapshot.conflicts_detected_total,
            snapshot.conflicts_resolved_total,
            snapshot.queue_size_current,
            snapshot.bytes_synced_total,
            snapshot.entries_synced_total,
            snapshot.offline_events_total,
            snapshot.online_events_total,
        )
    }
}

/// Timer for measuring sync operation duration
pub struct SyncOperationTimer {
    start: Instant,
}

impl SyncOperationTimer {
    fn new(start: Instant) -> Self {
        Self { start }
    }

    /// Record the duration of the operation
    pub fn record(self, success: bool) {
        let duration = self.start.elapsed();
        let duration_ms = duration.as_secs_f64() * 1000.0;
        
        histogram!("edge_sync_operation_duration_ms").record(duration_ms);
        
        if success {
            histogram!("edge_sync_success_duration_ms").record(duration_ms);
        } else {
            histogram!("edge_sync_failure_duration_ms").record(duration_ms);
        }
    }
}

/// Snapshot of current metrics
#[derive(Debug, Clone)]
pub struct MetricsSnapshot {
    pub sync_operations_total: u64,
    pub sync_success_total: u64,
    pub sync_failure_total: u64,
    pub conflicts_detected_total: u64,
    pub conflicts_resolved_total: u64,
    pub queue_size_current: u64,
    pub bytes_synced_total: u64,
    pub entries_synced_total: u64,
    pub offline_events_total: u64,
    pub online_events_total: u64,
}

impl MetricsSnapshot {
    pub fn success_rate(&self) -> f64 {
        if self.sync_operations_total == 0 {
            return 0.0;
        }
        self.sync_success_total as f64 / self.sync_operations_total as f64
    }

    pub fn conflict_resolution_rate(&self) -> f64 {
        if self.conflicts_detected_total == 0 {
            return 1.0;
        }
        self.conflicts_resolved_total as f64 / self.conflicts_detected_total as f64
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_metrics_recording() {
        let metrics = SyncMetrics::new();
        
        let timer = metrics.record_sync_start();
        std::thread::sleep(std::time::Duration::from_millis(10));
        timer.record(true);
        
        metrics.record_sync_success(1024, 5);
        
        let snapshot = metrics.snapshot();
        assert_eq!(snapshot.sync_operations_total, 1);
        assert_eq!(snapshot.sync_success_total, 1);
        assert_eq!(snapshot.bytes_synced_total, 1024);
        assert_eq!(snapshot.entries_synced_total, 5);
    }

    #[test]
    fn test_conflict_metrics() {
        let metrics = SyncMetrics::new();
        
        metrics.record_conflict_detected();
        metrics.record_conflict_detected();
        metrics.record_conflict_resolved("last_write_wins");
        
        let snapshot = metrics.snapshot();
        assert_eq!(snapshot.conflicts_detected_total, 2);
        assert_eq!(snapshot.conflicts_resolved_total, 1);
    }

    #[test]
    fn test_prometheus_export() {
        let metrics = SyncMetrics::new();
        metrics.record_sync_success(100, 1);
        
        let export = metrics.export_prometheus();
        assert!(export.contains("edge_sync_operations_total"));
        assert!(export.contains("edge_sync_success_total"));
        assert!(export.contains("# HELP"));
        assert!(export.contains("# TYPE"));
    }
}
