//! Vector Index Service - Milvus Client and HNSW Indexing

use std::sync::Arc;

pub mod config;
pub mod error;
pub mod milvus_client;
pub mod indexer;
pub mod search;
pub mod quantizer;
pub mod cache;
pub mod metrics;
pub mod api;
pub mod schema;

#[derive(Clone)]
pub struct IndexState {
    pub milvus: Arc<milvus_client::MilvusClient>,
    pub indexer: Arc<indexer::VectorIndexer>,
    pub cache: Arc<cache::VectorCache>,
}

pub fn create_state(config: &config::Config) -> Result<IndexState, error::Error> {
    let milvus = Arc::new(milvus_client::MilvusClient::new(&config.milvus_url)?);
    let indexer = Arc::new(indexer::VectorIndexer::new(config.index_type.clone()));
    let cache = Arc::new(cache::VectorCache::new(config.cache_size));
    
    Ok(IndexState {
        milvus,
        indexer,
        cache,
    })
}
