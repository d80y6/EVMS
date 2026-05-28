//! Configuration management for edge sync service

use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use crate::{ConflictResolutionStrategy, RetryPolicy};

/// Main configuration struct
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// Unique device identifier
    pub device_id: String,
    
    /// Path to storage backend
    pub storage_path: String,
    
    /// Network configuration
    pub network: NetworkConfig,
    
    /// Sync configuration
    pub sync: SyncConfig,
    
    /// Conflict resolution strategy
    pub conflict_strategy: ConflictResolutionStrategy,
    
    /// Retry policy for failed operations
    pub retry_policy: RetryPolicy,
    
    /// Maximum offline queue size
    pub max_queue_size: usize,
    
    /// Maximum replication log size
    pub max_replication_log_size: usize,
    
    /// TLS configuration
    pub tls: Option<TlsConfig>,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            device_id: generate_device_id(),
            storage_path: "./data/edge-sync".to_string(),
            network: NetworkConfig::default(),
            sync: SyncConfig::default(),
            conflict_strategy: ConflictResolutionStrategy::LastWriteWins,
            retry_policy: RetryPolicy::default(),
            max_queue_size: 10000,
            max_replication_log_size: 100000,
            tls: None,
        }
    }
}

impl Config {
    /// Load configuration from environment variables
    pub fn from_env() -> Result<Self, ConfigError> {
        let device_id = std::env::var("EDGE_SYNC__DEVICE_ID")
            .unwrap_or_else(|_| generate_device_id());
        
        let storage_path = std::env::var("EDGE_SYNC__STORAGE_PATH")
            .unwrap_or_else(|_| "./data/edge-sync".to_string());
        
        let grpc_port = std::env::var("EDGE_SYNC__GRPC_PORT")
            .unwrap_or_else(|_| "50051".to_string())
            .parse::<u16>()
            .map_err(|e| ConfigError::ParseError(format!("GRPC_PORT: {}", e)))?;
        
        let http_port = std::env::var("EDGE_SYNC__HTTP_PORT")
            .unwrap_or_else(|_| "8080".to_string())
            .parse::<u16>()
            .map_err(|e| ConfigError::ParseError(format!("HTTP_PORT: {}", e)))?;
        
        let sync_interval_secs = std::env::var("EDGE_SYNC__SYNC_INTERVAL_SECS")
            .unwrap_or_else(|_| "30".to_string())
            .parse::<u64>()
            .map_err(|e| ConfigError::ParseError(format!("SYNC_INTERVAL_SECS: {}", e)))?;
        
        let max_queue_size = std::env::var("EDGE_SYNC__MAX_QUEUE_SIZE")
            .unwrap_or_else(|_| "10000".to_string())
            .parse::<usize>()
            .map_err(|e| ConfigError::ParseError(format!("MAX_QUEUE_SIZE: {}", e)))?;
        
        let conflict_strategy = std::env::var("EDGE_SYNC__CONFLICT_STRATEGY")
            .unwrap_or_else(|_| "last_write_wins".to_string())
            .parse::<ConflictResolutionStrategy>()?;
        
        Ok(Self {
            device_id,
            storage_path,
            max_queue_size,
            network: NetworkConfig {
                grpc_port,
                http_port,
                bind_address: std::env::var("EDGE_SYNC__BIND_ADDRESS")
                    .unwrap_or_else(|_| "0.0.0.0".to_string()),
                central_server_url: std::env::var("EDGE_SYNC__CENTRAL_SERVER_URL")
                    .ok(),
            },
            sync: SyncConfig {
                interval_secs: sync_interval_secs,
                batch_size: std::env::var("EDGE_SYNC__BATCH_SIZE")
                    .unwrap_or_else(|_| "100".to_string())
                    .parse::<usize>()
                    .map_err(|e| ConfigError::ParseError(format!("BATCH_SIZE: {}", e)))?,
                compression_enabled: std::env::var("EDGE_SYNC__COMPRESSION_ENABLED")
                    .unwrap_or_else(|_| "true".to_string())
                    .parse::<bool>()
                    .unwrap_or(true),
                encryption_enabled: std::env::var("EDGE_SYNC__ENCRYPTION_ENABLED")
                    .unwrap_or_else(|_| "false".to_string())
                    .parse::<bool>()
                    .unwrap_or(false),
            },
            conflict_strategy,
            retry_policy: RetryPolicy::default(),
            max_replication_log_size: 100000,
            tls: TlsConfig::from_env().ok(),
        })
    }
    
    /// Validate configuration
    pub fn validate(&self) -> Result<(), ConfigError> {
        if self.device_id.is_empty() {
            return Err(ConfigError::ValidationError("device_id cannot be empty".to_string()));
        }
        
        if self.storage_path.is_empty() {
            return Err(ConfigError::ValidationError("storage_path cannot be empty".to_string()));
        }
        
        if self.network.grpc_port == 0 || self.network.http_port == 0 {
            return Err(ConfigError::ValidationError("ports cannot be zero".to_string()));
        }
        
        if self.sync.interval_secs == 0 {
            return Err(ConfigError::ValidationError("sync_interval_secs cannot be zero".to_string()));
        }
        
        Ok(())
    }
}

/// Network configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct NetworkConfig {
    /// gRPC server port
    pub grpc_port: u16,
    
    /// HTTP server port
    pub http_port: u16,
    
    /// Bind address
    pub bind_address: String,
    
    /// Central server URL for synchronization
    pub central_server_url: Option<String>,
}

impl Default for NetworkConfig {
    fn default() -> Self {
        Self {
            grpc_port: 50051,
            http_port: 8080,
            bind_address: "0.0.0.0".to_string(),
            central_server_url: None,
        }
    }
}

/// Sync configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SyncConfig {
    /// Sync interval in seconds
    pub interval_secs: u64,
    
    /// Batch size for sync operations
    pub batch_size: usize,
    
    /// Enable compression
    pub compression_enabled: bool,
    
    /// Enable encryption
    pub encryption_enabled: bool,
}

impl Default for SyncConfig {
    fn default() -> Self {
        Self {
            interval_secs: 30,
            batch_size: 100,
            compression_enabled: true,
            encryption_enabled: false,
        }
    }
}

/// TLS configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TlsConfig {
    pub cert_path: String,
    pub key_path: String,
    pub ca_cert_path: Option<String>,
}

impl TlsConfig {
    pub fn from_env() -> Option<Self> {
        let cert_path = std::env::var("EDGE_SYNC__TLS_CERT_PATH").ok()?;
        let key_path = std::env::var("EDGE_SYNC__TLS_KEY_PATH").ok()?;
        
        Some(Self {
            cert_path,
            key_path,
            ca_cert_path: std::env::var("EDGE_SYNC__TLS_CA_CERT_PATH").ok(),
        })
    }
}

/// Configuration errors
#[derive(Debug, thiserror::Error)]
pub enum ConfigError {
    #[error("Parse error: {0}")]
    ParseError(String),
    
    #[error("Validation error: {0}")]
    ValidationError(String),
    
    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),
}

impl From<ConfigError> for crate::EdgeSyncError {
    fn from(err: ConfigError) -> Self {
        crate::EdgeSyncError::ConfigError(err.to_string())
    }
}

impl std::str::FromStr for ConflictResolutionStrategy {
    type Err = ConfigError;
    
    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "last_write_wins" | "lww" => Ok(ConflictResolutionStrategy::LastWriteWins),
            "merge" => Ok(ConflictResolutionStrategy::Merge),
            "manual" => Ok(ConflictResolutionStrategy::Manual),
            "custom" => Ok(ConflictResolutionStrategy::Custom),
            _ => Err(ConfigError::ParseError(format!(
                "Invalid conflict strategy: {}. Valid values: last_write_wins, merge, manual, custom",
                s
            ))),
        }
    }
}

/// Generate a unique device ID
fn generate_device_id() -> String {
    use std::time::{SystemTime, UNIX_EPOCH};
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_millis();
    format!("edge-device-{}", timestamp)
}
