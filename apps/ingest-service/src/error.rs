//! Error types for the ingest service

use thiserror::Error;

/// Main error type for the ingest service
#[derive(Debug, Error)]
pub enum Error {
    /// Configuration error
    #[error("Configuration error: {0}")]
    Config(String),
    
    /// RTSP protocol error
    #[error("RTSP error: {0}")]
    Rtsp(String),
    
    /// RTP packet error
    #[error("RTP error: {0}")]
    Rtp(String),
    
    /// RTCP packet error
    #[error("RTCP error: {0}")]
    Rtcp(String),
    
    /// Muxer error
    #[error("Muxer error: {0}")]
    Muxer(String),
    
    /// Storage error
    #[error("Storage error: {0}")]
    Storage(String),
    
    /// WebRTC error
    #[error("WebRTC error: {0}")]
    Webrtc(String),
    
    /// Network error
    #[error("Network error: {0}")]
    Network(#[from] std::io::Error),
    
    /// Tokio channel error
    #[error("Channel error: {0}")]
    Channel(#[from] tokio::sync::broadcast::error::RecvError),
    
    /// Send error
    #[error("Send error: {0}")]
    Send(#[from] tokio::sync::broadcast::error::SendError<()>),
    
    /// JSON serialization error
    #[error("JSON error: {0}")]
    Json(#[from] serde_json::Error),
    
    /// Metrics error
    #[error("Metrics error: {0}")]
    Metrics(String),
    
    /// S3 error
    #[error("S3 error: {0}")]
    S3(String),
    
    /// Timeout error
    #[error("Operation timed out")]
    Timeout,
    
    /// Buffer overflow
    #[error("Buffer overflow: {0}")]
    BufferOverflow(String),
    
    /// Invalid state
    #[error("Invalid state: {0}")]
    InvalidState(String),
    
    /// Anyhow wrapped error
    #[error(transparent)]
    Other(#[from] anyhow::Error),
}

/// Result type alias
pub type Result<T> = std::result::Result<T, Error>;

impl From<rtp::Error> for Error {
    fn from(err: rtp::Error) -> Self {
        Error::Rtp(err.to_string())
    }
}

impl From<rtcp::Error> for Error {
    fn from(err: rtcp::Error) -> Self {
        Error::Rtcp(err.to_string())
    }
}
