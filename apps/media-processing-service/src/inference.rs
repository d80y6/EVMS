use crate::error::{Result, MediaProcessingError};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InferenceRequest {
    pub model_name: String,
    pub model_version: Option<String>,
    pub inputs: Vec<TensorInput>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TensorInput {
    pub name: String,
    pub shape: Vec<i64>,
    pub data: Vec<f32>,
    pub dtype: DataType,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum DataType {
    FP32,
    FP16,
    INT8,
    INT32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct InferenceResponse {
    pub model_name: String,
    pub outputs: Vec<TensorOutput>,
    pub latency_ms: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TensorOutput {
    pub name: String,
    pub shape: Vec<i64>,
    pub data: Vec<f32>,
}

pub struct InferenceClient {
    endpoint: String,
    client: reqwest::Client,
}

impl InferenceClient {
    pub fn new(endpoint: String) -> Self {
        InferenceClient {
            endpoint,
            client: reqwest::Client::new(),
        }
    }

    pub async fn infer(&self, request: InferenceRequest) -> Result<InferenceResponse> {
        let url = format!("{}/v2/models/{}/infer", self.endpoint, request.model_name);
        
        let response = self.client
            .post(&url)
            .json(&request)
            .send()
            .await
            .map_err(|e| MediaProcessingError::Inference(e.to_string()))?;

        if !response.status().is_success() {
            return Err(MediaProcessingError::Inference(format!("HTTP {}", response.status())));
        }

        let inference_response: InferenceResponse = response
            .json()
            .await
            .map_err(|e| MediaProcessingError::Inference(e.to_string()))?;

        Ok(inference_response)
    }
}
