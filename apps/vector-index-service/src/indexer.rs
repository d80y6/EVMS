use crate::error::{Error, Result};

#[derive(Clone)]
pub struct VectorIndexer {
    index_type: String,
}

impl VectorIndexer {
    pub fn new(index_type: String) -> Self {
        Self { index_type }
    }

    pub async fn build(&self, vectors: &[Vec<f32>]) -> Result<IndexHandle> {
        Ok(IndexHandle { id: uuid::Uuid::new_v4().to_string() })
    }

    pub async fn insert(&self, handle: &IndexHandle, vectors: &[Vec<f32>], ids: &[i64]) -> Result<()> {
        Ok(())
    }

    pub async fn search(&self, handle: &IndexHandle, query: &[f32], k: usize) -> Result<Vec<(i64, f32)>> {
        Ok(vec![])
    }
}

#[derive(Debug, Clone)]
pub struct IndexHandle {
    pub id: String,
}
