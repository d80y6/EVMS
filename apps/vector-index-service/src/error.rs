use thiserror::Error;

#[derive(Error, Debug)]
pub enum Error {
    #[error("Connection error: {0}")]
    Connection(String),
    #[error("Index error: {0}")]
    Index(String),
    #[error("Search error: {0}")]
    Search(String),
    #[error("Quantization error: {0}")]
    Quantization(String),
    #[error("Cache error: {0}")]
    Cache(String),
    #[error("Invalid input: {0}")]
    InvalidInput(String),
    #[error("Internal error: {0}")]
    Internal(String),
}

impl From<milvus_client::Error> for Error {
    fn from(err: milvus_client::Error) -> Self {
        Error::Connection(err.to_string())
    }
}

pub type Result<T> = std::result::Result<T, Error>;
