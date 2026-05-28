use std::collections::VecDeque;
use tokio::sync::Mutex;
use crate::{InferenceRequest, error::Result};

/// Dynamic batching engine with timeout-based flushing
pub struct Batcher {
    max_batch_size: usize,
    timeout_ms: u64,
    queue: Mutex<VecDeque<PendingRequest>>,
}

struct PendingRequest {
    request: InferenceRequest,
    timestamp: std::time::Instant,
}

impl Batcher {
    pub fn new(max_batch_size: usize, timeout_ms: u64) -> Self {
        Self {
            max_batch_size,
            timeout_ms,
            queue: Mutex::new(VecDeque::new()),
        }
    }

    pub async fn add(&self, request: InferenceRequest) -> Result<()> {
        let mut queue = self.queue.lock().await;
        queue.push_back(PendingRequest {
            request,
            timestamp: std::time::Instant::now(),
        });
        Ok(())
    }

    pub async fn get_batch(&self) -> Result<Vec<InferenceRequest>> {
        let mut queue = self.queue.lock().await;
        let mut batch = Vec::new();
        let now = std::time::Instant::now();

        while batch.len() < self.max_batch_size {
            if let Some(front) = queue.front() {
                if front.timestamp.elapsed().as_millis() >= self.timeout_ms as u128 {
                    // Timeout reached, flush whatever we have
                    break;
                }
                
                if let Some(pending) = queue.pop_front() {
                    batch.push(pending.request);
                }
            } else {
                break;
            }
        }

        Ok(batch)
    }

    pub async fn queue_size(&self) -> usize {
        let queue = self.queue.lock().await;
        queue.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::{InputTensor, TensorDataType, TensorData, InferenceParameters};

    #[tokio::test]
    async fn test_batch_creation() {
        let batcher = Batcher::new(4, 100);
        
        for i in 0..3 {
            let req = InferenceRequest {
                id: format!("req-{}", i),
                model_name: "test-model".to_string(),
                model_version: None,
                inputs: vec![],
                parameters: InferenceParameters::default(),
            };
            batcher.add(req).await.unwrap();
        }

        let batch = batcher.get_batch().await.unwrap();
        assert_eq!(batch.len(), 3);
    }

    #[tokio::test]
    async fn test_batch_timeout() {
        let batcher = Batcher::new(4, 10); // 10ms timeout
        
        let req = InferenceRequest {
            id: "req-1".to_string(),
            model_name: "test-model".to_string(),
            model_version: None,
            inputs: vec![],
            parameters: InferenceParameters::default(),
        };
        batcher.add(req).await.unwrap();

        tokio::time::sleep(tokio::time::Duration::from_millis(20)).await;

        let batch = batcher.get_batch().await.unwrap();
        assert_eq!(batch.len(), 1);
    }
}
