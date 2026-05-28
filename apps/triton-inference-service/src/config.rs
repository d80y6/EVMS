use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub triton_url: String,
    pub http_port: u16,
    pub grpc_port: u16,
    pub max_batch_size: usize,
    pub batch_timeout_ms: u64,
    pub max_concurrent_requests: usize,
    pub model_repository_path: String,
    pub default_model: Option<String>,
    pub gpu_memory_fraction: f32,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            triton_url: std::env::var("TRITON_URL").unwrap_or_else(|_| "localhost:8001".to_string()),
            http_port: std::env::var("TRITON_HTTP_PORT")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(3000),
            grpc_port: std::env::var("TRITON_GRPC_PORT")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(3001),
            max_batch_size: std::env::var("TRITON_MAX_BATCH_SIZE")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(32),
            batch_timeout_ms: std::env::var("TRITON_BATCH_TIMEOUT_MS")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(100),
            max_concurrent_requests: std::env::var("TRITON_MAX_CONCURRENT_REQUESTS")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(1000),
            model_repository_path: std::env::var("TRITON_MODEL_REPO")
                .unwrap_or_else(|_| "/models".to_string()),
            default_model: std::env::var("TRITON_DEFAULT_MODEL").ok(),
            gpu_memory_fraction: std::env::var("TRITON_GPU_MEMORY_FRACTION")
                .ok()
                .and_then(|s| s.parse().ok())
                .unwrap_or(0.8),
        }
    }
}

impl Config {
    pub fn load() -> Result<Self, Box<dyn std::error::Error>> {
        Ok(Self::default())
    }
}
