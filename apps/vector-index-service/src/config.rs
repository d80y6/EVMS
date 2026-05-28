use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    pub milvus_url: String,
    pub http_port: u16,
    pub grpc_port: u16,
    pub index_type: String,
    pub cache_size: usize,
    pub quantization_enabled: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            milvus_url: std::env::var("VECTOR_MILVUS_URL").unwrap_or_else(|_| "localhost:19530".to_string()),
            http_port: std::env::var("VECTOR_HTTP_PORT").ok().and_then(|s| s.parse().ok()).unwrap_or(3000),
            grpc_port: std::env::var("VECTOR_GRPC_PORT").ok().and_then(|s| s.parse().ok()).unwrap_or(3001),
            index_type: std::env::var("VECTOR_INDEX_TYPE").unwrap_or_else(|_| "HNSW".to_string()),
            cache_size: std::env::var("VECTOR_CACHE_SIZE").ok().and_then(|s| s.parse().ok()).unwrap_or(10000),
            quantization_enabled: std::env::var("VECTOR_QUANTIZATION_ENABLED").ok().map(|s| s == "true").unwrap_or(false),
        }
    }
}

impl Config {
    pub fn load() -> Result<Self, Box<dyn std::error::Error>> {
        Ok(Self::default())
    }
}
