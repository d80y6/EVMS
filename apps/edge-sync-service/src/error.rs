//! Error types for edge sync service

use thiserror::Error;

/// Main error type for edge sync operations
#[derive(Debug, Error)]
pub enum EdgeSyncError {
    #[error("Storage error: {0}")]
    StorageError(String),

    #[error("Network error: {0}")]
    NetworkError(String),

    #[error("Serialization error: {0}")]
    SerializationError(String),

    #[error("Conflict resolution error: {0}")]
    ConflictError(String),

    #[error("Queue error: {0}")]
    QueueError(String),

    #[error("Replication error: {0}")]
    ReplicationError(String),

    #[error("Vector clock error: {0}")]
    VectorClockError(String),

    #[error("Configuration error: {0}")]
    ConfigError(String),

    #[error("Compression error: {0}")]
    CompressionError(String),

    #[error("Encryption error: {0}")]
    EncryptionError(String),

    #[error("gRPC error: {0}")]
    GrpcError(String),

    #[error("Database error: {0}")]
    DatabaseError(String),

    #[error("IO error: {0}")]
    IoError(#[from] std::io::Error),

    #[error("Channel send error: {0}")]
    ChannelSendError(String),

    #[error("Channel recv error: {0}")]
    ChannelRecvError(String),

    #[error("Timeout error: {0}")]
    TimeoutError(String),

    #[error("Not found: {0}")]
    NotFound(String),

    #[error("Invalid state: {0}")]
    InvalidState(String),

    #[error("Permission denied: {0}")]
    PermissionDenied(String),

    #[error("Rate limited: {0}")]
    RateLimited(String),

    #[error("Unknown error: {0}")]
    Unknown(String),
}

impl From<tonic::Status> for EdgeSyncError {
    fn from(status: tonic::Status) -> Self {
        match status.code() {
            tonic::Code::NotFound => EdgeSyncError::NotFound(status.message().to_string()),
            tonic::Code::AlreadyExists => EdgeSyncError::StorageError(status.message().to_string()),
            tonic::Code::PermissionDenied => EdgeSyncError::PermissionDenied(status.message().to_string()),
            tonic::Code::ResourceExhausted => EdgeSyncError::RateLimited(status.message().to_string()),
            tonic::Code::InvalidArgument => EdgeSyncError::InvalidState(status.message().to_string()),
            tonic::Code::DeadlineExceeded => EdgeSyncError::TimeoutError(status.message().to_string()),
            _ => EdgeSyncError::GrpcError(status.message().to_string()),
        }
    }
}

impl From<serde_json::Error> for EdgeSyncError {
    fn from(err: serde_json::Error) -> Self {
        EdgeSyncError::SerializationError(err.to_string())
    }
}

impl From<bincode::Error> for EdgeSyncError {
    fn from(err: bincode::Error) -> Self {
        EdgeSyncError::SerializationError(err.to_string())
    }
}

impl From<prost::EncodeError> for EdgeSyncError {
    fn from(err: prost::EncodeError) -> Self {
        EdgeSyncError::SerializationError(err.to_string())
    }
}

impl From<prost::DecodeError> for EdgeSyncError {
    fn from(err: prost::DecodeError) -> Self {
        EdgeSyncError::SerializationError(err.to_string())
    }
}

impl<T> From<tokio::sync::mpsc::error::SendError<T>> for EdgeSyncError {
    fn from(err: tokio::sync::mpsc::error::SendError<T>) -> Self {
        EdgeSyncError::ChannelSendError(err.to_string())
    }
}

impl From<tokio::sync::mpsc::error::RecvError> for EdgeSyncError {
    fn from(err: tokio::sync::mpsc::error::RecvError) -> Self {
        EdgeSyncError::ChannelRecvError(err.to_string())
    }
}

impl From<tokio::time::error::Elapsed> for EdgeSyncError {
    fn from(err: tokio::time::error::Elapsed) -> Self {
        EdgeSyncError::TimeoutError(err.to_string())
    }
}

impl From<sqlx::Error> for EdgeSyncError {
    fn from(err: sqlx::Error) -> Self {
        EdgeSyncError::DatabaseError(err.to_string())
    }
}

impl From<rocksdb::Error> for EdgeSyncError {
    fn from(err: rocksdb::Error) -> Self {
        EdgeSyncError::StorageError(err.to_string())
    }
}

/// Result type alias
pub type Result<T> = std::result::Result<T, EdgeSyncError>;
