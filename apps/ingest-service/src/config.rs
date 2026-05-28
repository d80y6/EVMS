//! Configuration management for the ingest service

use serde::{Deserialize, Serialize};
use std::path::PathBuf;
use std::time::Duration;

/// Main configuration structure
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Config {
    /// RTSP server port
    #[serde(default = "default_rtsp_port")]
    pub rtsp_port: u16,
    
    /// RTP buffer size in packets
    #[serde(default = "default_rtp_buffer_size")]
    pub rtp_buffer_size: usize,
    
    /// Maximum RTP packet latency in milliseconds
    #[serde(default = "default_rtp_max_latency_ms")]
    pub rtp_max_latency_ms: u64,
    
    /// Segment duration for fMP4 muxing in milliseconds
    #[serde(default = "default_segment_duration_ms")]
    pub segment_duration_ms: u64,
    
    /// Path to store initialization segments
    pub init_segment_path: Option<PathBuf>,
    
    /// S3 bucket name
    pub s3_bucket: String,
    
    /// S3 region
    pub s3_region: String,
    
    /// S3 endpoint (for compatible services)
    pub s3_endpoint: Option<String>,
    
    /// S3 access key
    pub s3_access_key: Option<String>,
    
    /// S3 secret key
    pub s3_secret_key: Option<String>,
    
    /// Metrics bind address
    #[serde(default = "default_metrics_bind_addr")]
    pub metrics_bind_addr: String,
    
    /// API server bind address
    #[serde(default = "default_api_bind_addr")]
    pub api_bind_addr: String,
    
    /// WebRTC ICE servers
    #[serde(default)]
    pub ice_servers: Vec<String>,
    
    /// Enable debug logging
    #[serde(default)]
    pub debug: bool,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            rtsp_port: default_rtsp_port(),
            rtp_buffer_size: default_rtp_buffer_size(),
            rtp_max_latency_ms: default_rtp_max_latency_ms(),
            segment_duration_ms: default_segment_duration_ms(),
            init_segment_path: None,
            s3_bucket: String::new(),
            s3_region: String::from("us-east-1"),
            s3_endpoint: None,
            s3_access_key: None,
            s3_secret_key: None,
            metrics_bind_addr: default_metrics_bind_addr(),
            api_bind_addr: default_api_bind_addr(),
            ice_servers: Vec::new(),
            debug: false,
        }
    }
}

fn default_rtsp_port() -> u16 {
    554
}

fn default_rtp_buffer_size() -> usize {
    1024
}

fn default_rtp_max_latency_ms() -> u64 {
    500
}

fn default_segment_duration_ms() -> u64 {
    2000
}

fn default_metrics_bind_addr() -> String {
    "0.0.0.0:9090".to_string()
}

fn default_api_bind_addr() -> String {
    "0.0.0.0:8080".to_string()
}

impl Config {
    /// Load configuration from environment and config files
    pub fn load() -> Result<Self, crate::Error> {
        // Load from .env file if present
        let _ = dotenvy::dotenv();
        
        // Build configuration from environment variables
        let config = config::Config::builder()
            .add_source(config::Environment::with_prefix("INGEST").separator("__"))
            .build()
            .map_err(|e| crate::Error::Config(e.to_string()))?;
        
        let mut cfg = Config::default();
        
        if let Ok(port) = config.get::<u16>("rtsp_port") {
            cfg.rtsp_port = port;
        }
        
        if let Ok(size) = config.get::<usize>("rtp_buffer_size") {
            cfg.rtp_buffer_size = size;
        }
        
        if let Ok(latency) = config.get::<u64>("rtp_max_latency_ms") {
            cfg.rtp_max_latency_ms = latency;
        }
        
        if let Ok(duration) = config.get::<u64>("segment_duration_ms") {
            cfg.segment_duration_ms = duration;
        }
        
        if let Ok(path) = config.get::<String>("init_segment_path") {
            cfg.init_segment_path = Some(PathBuf::from(path));
        }
        
        if let Ok(bucket) = config.get::<String>("s3_bucket") {
            cfg.s3_bucket = bucket;
        }
        
        if let Ok(region) = config.get::<String>("s3_region") {
            cfg.s3_region = region;
        }
        
        if let Ok(endpoint) = config.get::<String>("s3_endpoint") {
            cfg.s3_endpoint = Some(endpoint);
        }
        
        if let Ok(access_key) = config.get::<String>("s3_access_key") {
            cfg.s3_access_key = Some(access_key);
        }
        
        if let Ok(secret_key) = config.get::<String>("s3_secret_key") {
            cfg.s3_secret_key = Some(secret_key);
        }
        
        if let Ok(addr) = config.get::<String>("metrics_bind_addr") {
            cfg.metrics_bind_addr = addr;
        }
        
        if let Ok(addr) = config.get::<String>("api_bind_addr") {
            cfg.api_bind_addr = addr;
        }
        
        if let Ok(debug) = config.get::<bool>("debug") {
            cfg.debug = debug;
        }
        
        Ok(cfg)
    }
    
    /// Get segment duration as Duration
    pub fn segment_duration(&self) -> Duration {
        Duration::from_millis(self.segment_duration_ms)
    }
    
    /// Get max RTP latency as Duration
    pub fn rtp_max_latency(&self) -> Duration {
        Duration::from_millis(self.rtp_max_latency_ms)
    }
}
