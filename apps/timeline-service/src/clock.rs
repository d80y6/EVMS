use chrono::{DateTime, Utc, NaiveDateTime};
use serde::{Deserialize, Serialize};
use std::sync::atomic::{AtomicU64, Ordering};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ClockState {
    pub physical_time_ms: i64,
    pub logical_counter: u64,
    pub last_update: DateTime<Utc>,
}

pub struct HybridLogicalClock {
    physical_time: AtomicU64,
    logical_counter: AtomicU64,
}

impl HybridLogicalClock {
    pub fn new() -> Self {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        HybridLogicalClock {
            physical_time: AtomicU64::new(now),
            logical_counter: AtomicU64::new(0),
        }
    }

    pub fn now(&self) -> (i64, u64) {
        let physical = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as u64;
        
        let prev_physical = self.physical_time.load(Ordering::Relaxed);
        let prev_logical = self.logical_counter.load(Ordering::Relaxed);
        
        let (new_physical, new_logical) = if physical > prev_physical {
            self.physical_time.store(physical, Ordering::Relaxed);
            self.logical_counter.store(0, Ordering::Relaxed);
            (physical, 0u64)
        } else if physical == prev_physical {
            let counter = prev_logical + 1;
            self.logical_counter.store(counter, Ordering::Relaxed);
            (prev_physical, counter)
        } else {
            let counter = prev_logical + 1;
            self.physical_time.store(prev_physical, Ordering::Relaxed);
            self.logical_counter.store(counter, Ordering::Relaxed);
            (prev_physical, counter)
        };
        
        (new_physical as i64, new_logical)
    }

    pub fn update(&self, remote_physical: i64, remote_logical: u64) -> (i64, u64) {
        let local_physical = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .unwrap()
            .as_millis() as i64;
        
        let max_physical = local_physical.max(remote_physical);
        let new_logical = if max_physical > local_physical && max_physical > remote_physical {
            0
        } else if max_physical == local_physical && max_physical == remote_physical {
            remote_logical + 1
        } else {
            1
        };
        
        self.physical_time.store(max_physical as u64, Ordering::Relaxed);
        self.logical_counter.store(new_logical, Ordering::Relaxed);
        
        (max_physical, new_logical)
    }

    pub fn get_state(&self) -> ClockState {
        let (physical, logical) = self.now();
        ClockState {
            physical_time_ms: physical,
            logical_counter: logical,
            last_update: Utc::now(),
        }
    }
}

impl Default for HybridLogicalClock {
    fn default() -> Self {
        Self::new()
    }
}

pub struct TimeSync {
    clock: HybridLogicalClock,
    offset_ms: f64,
    rtt_ms: f64,
}

impl TimeSync {
    pub fn new() -> Self {
        TimeSync {
            clock: HybridLogicalClock::new(),
            offset_ms: 0.0,
            rtt_ms: 0.0,
        }
    }

    pub fn update_offset(&mut self, offset_ms: f64, rtt_ms: f64) {
        self.offset_ms = offset_ms;
        self.rtt_ms = rtt_ms;
    }

    pub fn get_corrected_time(&self) -> i64 {
        let (physical, _) = self.clock.now();
        (physical as f64 + self.offset_ms) as i64
    }

    pub fn get_clock(&self) -> &HybridLogicalClock {
        &self.clock
    }
}

impl Default for TimeSync {
    fn default() -> Self {
        Self::new()
    }
}
