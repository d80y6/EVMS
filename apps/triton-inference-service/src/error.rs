use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    #[error("Triton connection error: {0}")]
    Connection(String),

    #[error("Model error: {0}")]
    Model(String),

    #[error("Batching error: {0}")]
    Batching(String),

    #[error("Preprocessing error: {0}")]
    Preprocessing(String),

    #[error("Postprocessing error: {0}")]
    Postprocessing(String),

    #[error("Tensor error: {0}")]
    Tensor(String),

    #[error("Timeout error: {0}")]
    Timeout(String),

    #[error("Invalid input: {0}")]
    InvalidInput(String),

    #[error("Internal error: {0}")]
    Internal(String),
}

impl From<tonic::Status> for Error {
    fn from(status: tonic::Status) -> Self {
        Error::Connection(status.message().to_string())
    }
}

impl From<std::io::Error> for Error {
    fn from(err: std::io::Error) -> Self {
        Error::Internal(err.to_string())
    }
}

pub type Result<T> = std::result::Result<T, Error>;
