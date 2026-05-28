use crate::error::{Error, Result};

#[derive(Clone)]
pub struct MilvusClient {
    url: String,
}

impl MilvusClient {
    pub fn new(url: &str) -> Result<Self> {
        Ok(Self { url: url.to_string() })
    }

    pub async fn is_ready(&self) -> bool {
        true
    }

    pub async fn create_collection(&self, name: &str, dimension: i64) -> Result<()> {
        Ok(())
    }

    pub async fn insert(&self, collection: &str, vectors: &[Vec<f32>], ids: &[i64]) -> Result<()> {
        Ok(())
    }

    pub async fn search(&self, collection: &str, query: &[f32], k: i64) -> Result<Vec<SearchResult>> {
        Ok(vec![])
    }

    pub async fn drop_collection(&self, name: &str) -> Result<()> {
        Ok(())
    }
}

#[derive(Debug, Clone)]
pub struct SearchResult {
    pub id: i64,
    pub score: f32,
    pub vector: Option<Vec<f32>>,
}
