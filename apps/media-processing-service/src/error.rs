use thiserror::Error;

#[derive(Error, Debug)]
pub enum MediaProcessingError {
    #[error("GStreamer error: {0}")]
    GStreamer(String),
    
    #[error("Pipeline not found: {0}")]
    PipelineNotFound(String),
    
    #[error("Pipeline already exists: {0}")]
    PipelineExists(String),
    
    #[error("Pipeline state error: {0}")]
    PipelineState(String),
    
    #[error("Inference error: {0}")]
    Inference(String),
    
    #[error("Preprocessing error: {0}")]
    Preprocessing(String),
    
    #[error("Postprocessing error: {0}")]
    Postprocessing(String),
    
    #[error("Configuration error: {0}")]
    Config(String),
    
    #[error("IO error: {0}")]
    Io(#[from] std::io::Error),
}

pub type Result<T> = std::result::Result<T, MediaProcessingError>;
