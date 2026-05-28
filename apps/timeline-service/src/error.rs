use thiserror::Error;

#[derive(Error, Debug)]
pub enum TimelineError {
    #[error("Clock synchronization error: {0}")]
    ClockSync(String),
    
    #[error("Alignment error: {0}")]
    Alignment(String),
    
    #[error("Segment not found: {0}")]
    SegmentNotFound(String),
    
    #[error("Segment already exists: {0}")]
    SegmentExists(String),
    
    #[error("Invalid timestamp: {0}")]
    InvalidTimestamp(String),
    
    #[error("Configuration error: {0}")]
    Config(String),
    
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
}

pub type Result<T> = std::result::Result<T, TimelineError>;
