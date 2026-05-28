//! Triton Inference Service - Async GPU Inference Engine
//! 
//! Provides asynchronous inference with dynamic batching, model management,
//! and GPU scheduling for video analytics workloads.

pub mod config;
pub mod error;
pub mod client;
pub mod batcher;
pub mod preprocess;
pub mod postprocess;
pub mod scheduler;
pub mod metrics;
pub mod api;
pub mod models;

use std::sync::Arc;
use dashmap::DashMap;
use tokio::sync::RwLock;

use crate::models::ModelRegistry;
use crate::batcher::Batcher;
use crate::client::TritonClient;
use crate::scheduler::RequestScheduler;

#[derive(Clone)]
pub struct InferenceState {
    pub client: Arc<TritonClient>,
    pub batcher: Arc<Batcher>,
    pub scheduler: Arc<RequestScheduler>,
    pub models: Arc<ModelRegistry>,
    pub active_requests: Arc<DashMap<String, ActiveRequest>>,
}

#[derive(Debug, Clone)]
pub struct ActiveRequest {
    pub id: String,
    pub model_name: String,
    pub created_at: chrono::DateTime<chrono::Utc>,
    pub status: RequestStatus,
}

#[derive(Debug, Clone, PartialEq)]
pub enum RequestStatus {
    Queued,
    Processing,
    Completed,
    Failed(String),
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct InferenceRequest {
    pub id: String,
    pub model_name: String,
    pub model_version: Option<String>,
    pub inputs: Vec<InputTensor>,
    pub parameters: InferenceParameters,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct InputTensor {
    pub name: String,
    pub datatype: TensorDataType,
    pub shape: Vec<i64>,
    pub data: TensorData,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub enum TensorDataType {
    Fp32,
    Fp16,
    Int8,
    Int32,
    Uint8,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub enum TensorData {
    Fp32(Vec<f32>),
    Fp16(Vec<u16>),
    Int8(Vec<i8>),
    Int32(Vec<i32>),
    Uint8(Vec<u8>),
}

#[derive(Debug, Clone, Default, serde::Serialize, serde::Deserialize)]
pub struct InferenceParameters {
    pub priority: u32,
    pub timeout_ms: Option<u64>,
    pub sequence_id: Option<u64>,
    pub sequence_start: bool,
    pub sequence_end: bool,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct InferenceResponse {
    pub id: String,
    pub model_name: String,
    pub model_version: String,
    pub outputs: Vec<OutputTensor>,
    pub parameters: ResponseParameters,
}

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct OutputTensor {
    pub name: String,
    pub datatype: TensorDataType,
    pub shape: Vec<i64>,
    pub data: TensorData,
}

#[derive(Debug, Clone, Default, serde::Serialize, serde::Deserialize)]
pub struct ResponseParameters {
    pub inference_time_ms: f64,
    pub queue_time_ms: f64,
    pub total_time_ms: f64,
}

pub fn create_state(config: &config::Config) -> Result<InferenceState, error::Error> {
    let client = Arc::new(TritonClient::new(&config.triton_url)?);
    let batcher = Arc::new(Batcher::new(config.max_batch_size, config.batch_timeout_ms));
    let scheduler = Arc::new(RequestScheduler::new(config.max_concurrent_requests));
    let models = Arc::new(ModelRegistry::new());
    
    Ok(InferenceState {
        client,
        batcher,
        scheduler,
        models,
        active_requests: Arc::new(DashMap::new()),
    })
}
