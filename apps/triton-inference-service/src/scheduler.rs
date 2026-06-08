use std::sync::atomic::{AtomicUsize, Ordering};
use tokio::sync::Semaphore;
use crate::error::{Error, Result};

/// Request scheduler with rate limiting and concurrency control
pub struct RequestScheduler {
    max_concurrent: usize,
    semaphore: Semaphore,
    active_count: AtomicUsize,
    total_processed: AtomicUsize,
}

impl RequestScheduler {
    pub fn new(max_concurrent: usize) -> Self {
        Self {
            max_concurrent,
            semaphore: Semaphore::new(max_concurrent),
            active_count: AtomicUsize::new(0),
            total_processed: AtomicUsize::new(0),
        }
    }

    pub async fn acquire(&self) -> Result<PermitGuard> {
        let permit = self.semaphore.acquire().await
            .map_err(|e| Error::Internal(format!("Semaphore error: {}", e)))?;
        
        self.active_count.fetch_add(1, Ordering::SeqCst);
        
        Ok(PermitGuard {
            scheduler: self,
            _permit: permit,
        })
    }

    pub fn active_count(&self) -> usize {
        self.active_count.load(Ordering::Relaxed)
    }

    pub fn total_processed(&self) -> usize {
        self.total_processed.load(Ordering::Relaxed)
    }

    pub fn available_slots(&self) -> usize {
        self.max_concurrent - self.active_count.load(Ordering::Relaxed)
    }

    fn release(&self) {
        self.active_count.fetch_sub(1, Ordering::SeqCst);
        self.total_processed.fetch_add(1, Ordering::SeqCst);
    }
}

pub struct PermitGuard<'a> {
    scheduler: &'a RequestScheduler,
    _permit: tokio::sync::SemaphorePermit<'a>,
}

impl<'a> Drop for PermitGuard<'a> {
    fn drop(&mut self) {
        self.scheduler.release();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn test_concurrency_limit() {
        let scheduler = RequestScheduler::new(2);
        
        assert_eq!(scheduler.available_slots(), 2);
        
        let permit1 = scheduler.acquire().await.unwrap();
        assert_eq!(scheduler.active_count(), 1);
        assert_eq!(scheduler.available_slots(), 1);
        
        let permit2 = scheduler.acquire().await.unwrap();
        assert_eq!(scheduler.active_count(), 2);
        assert_eq!(scheduler.available_slots(), 0);
        
        drop(permit1);
        assert_eq!(scheduler.active_count(), 1);
        assert_eq!(scheduler.total_processed(), 1);
        
        drop(permit2);
        assert_eq!(scheduler.active_count(), 0);
        assert_eq!(scheduler.total_processed(), 2);
    }

    #[tokio::test]
    async fn test_semaphore_blocking() {
        let scheduler = RequestScheduler::new(1);
        
        let _permit1 = scheduler.acquire().await.unwrap();
        
        // Try to acquire another permit with timeout
        let result = tokio::time::timeout(
            std::time::Duration::from_millis(50),
            scheduler.acquire()
        ).await;
        
        assert!(result.is_err()); // Should timeout
    }
}
