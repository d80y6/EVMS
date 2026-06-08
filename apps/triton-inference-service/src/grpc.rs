use std::sync::Arc;
use tonic::{Request, Response, Status};

use crate::{InferenceState, InferenceRequest, InputTensor, TensorDataType, TensorData, InferenceParameters};
pub use inference::inference_service_server::{InferenceService, InferenceServiceServer};

pub mod inference {
    tonic::include_proto!("inference");
}

use inference::{
    InferRequest, InferResponse, TensorOutput,
    HealthRequest, HealthResponse,
    ListModelsRequest, ListModelsResponse, ModelInfo,
};

#[derive(Clone)]
pub struct TritonGrpcService {
    state: Arc<InferenceState>,
}

impl TritonGrpcService {
    pub fn new(state: Arc<InferenceState>) -> Self {
        Self { state }
    }
}

#[tonic::async_trait]
impl InferenceService for TritonGrpcService {
    async fn infer(&self, request: Request<InferRequest>) -> Result<Response<InferResponse>, Status> {
        let req = request.into_inner();

        let mut inputs = Vec::with_capacity(req.inputs.len());
        for t in &req.inputs {
            let dtype = match t.datatype.to_lowercase().as_str() {
                "fp32" => TensorDataType::Fp32,
                "fp16" => TensorDataType::Fp16,
                "int8" => TensorDataType::Int8,
                "int32" => TensorDataType::Int32,
                "uint8" => TensorDataType::Uint8,
                _ => return Err(Status::invalid_argument(format!("unsupported datatype: {}", t.datatype))),
            };

            let data = match dtype {
                TensorDataType::Fp32 => {
                    let bytes = &t.data;
                    if bytes.len() % 4 != 0 {
                        return Err(Status::invalid_argument("FP32 data length not multiple of 4"));
                    }
                    let vals: Vec<f32> = bytes.chunks(4).map(|c| {
                        f32::from_le_bytes([c[0], c[1], c[2], c[3]])
                    }).collect();
                    TensorData::Fp32(vals)
                }
                TensorDataType::Uint8 => TensorData::Uint8(t.data.clone()),
                TensorDataType::Int8 => TensorData::Int8(t.data.iter().map(|&b| b as i8).collect()),
                TensorDataType::Int32 => {
                    let bytes = &t.data;
                    if bytes.len() % 4 != 0 {
                        return Err(Status::invalid_argument("INT32 data length not multiple of 4"));
                    }
                    let vals: Vec<i32> = bytes.chunks(4).map(|c| {
                        i32::from_le_bytes([c[0], c[1], c[2], c[3]])
                    }).collect();
                    TensorData::Int32(vals)
                }
                TensorDataType::Fp16 => {
                    let bytes = &t.data;
                    if bytes.len() % 2 != 0 {
                        return Err(Status::invalid_argument("FP16 data length not multiple of 2"));
                    }
                    let vals: Vec<u16> = bytes.chunks(2).map(|c| {
                        u16::from_le_bytes([c[0], c[1]])
                    }).collect();
                    TensorData::Fp16(vals)
                }
            };

            inputs.push(InputTensor {
                name: t.name.clone(),
                datatype: dtype,
                shape: t.shape.clone(),
                data,
            });
        }

        let params = InferenceParameters {
            priority: 0,
            timeout_ms: req.parameters.get("timeout_ms").and_then(|v| v.parse().ok()),
            sequence_id: req.parameters.get("sequence_id").and_then(|v| v.parse().ok()),
            sequence_start: req.parameters.get("sequence_start").map(|v| v == "true").unwrap_or(false),
            sequence_end: req.parameters.get("sequence_end").map(|v| v == "true").unwrap_or(false),
        };

        let inference_req = InferenceRequest {
            id: uuid::Uuid::new_v4().to_string(),
            model_name: req.model_name.clone(),
            model_version: Some(req.model_version),
            inputs,
            parameters: params,
        };

        // Acquire scheduler permit
        let _permit = self.state.scheduler.acquire().await
            .map_err(|e| Status::internal(format!("scheduler error: {}", e)))?;

        // Add to batcher and process
        self.state.batcher.add(inference_req).await
            .map_err(|e| Status::internal(format!("batch error: {}", e)))?;

        let batch = self.state.batcher.get_batch().await
            .map_err(|e| Status::internal(format!("batch error: {}", e)))?;

        if batch.is_empty() {
            return Err(Status::internal("batch processing returned empty"));
        }

        // Perform inference through existing pipeline
        let response = self.state.client.infer(batch.into_iter().next().unwrap()).await
            .map_err(|e| Status::internal(format!("inference error: {}", e)))?;

        let mut outputs = Vec::new();
        for out in &response.outputs {
            let data_bytes = match &out.data {
                TensorData::Fp32(vals) => vals.iter().flat_map(|v| v.to_le_bytes()).collect(),
                TensorData::Uint8(vals) => vals.clone(),
                TensorData::Int8(vals) => vals.iter().map(|&v| v as u8).collect(),
                TensorData::Int32(vals) => vals.iter().flat_map(|v| v.to_le_bytes()).collect(),
                TensorData::Fp16(vals) => vals.iter().flat_map(|v| v.to_le_bytes()).collect(),
            };

            let dtype_str = match &out.datatype {
                TensorDataType::Fp32 => "FP32",
                TensorDataType::Fp16 => "FP16",
                TensorDataType::Int8 => "INT8",
                TensorDataType::Int32 => "INT32",
                TensorDataType::Uint8 => "UINT8",
            };

            outputs.push(TensorOutput {
                name: out.name.clone(),
                datatype: dtype_str.to_string(),
                shape: out.shape.clone(),
                data: data_bytes,
            });
        }

        crate::metrics::increment_request_counter("success");
        crate::metrics::increment_model_inference(&response.model_name);

        Ok(Response::new(InferResponse {
            id: response.id,
            model_name: response.model_name,
            model_version: response.model_version,
            outputs,
            inference_time_ms: response.parameters.total_time_ms,
        }))
    }

    async fn health(&self, _request: Request<HealthRequest>) -> Result<Response<HealthResponse>, Status> {
        let healthy = self.state.client.is_ready().await;
        Ok(Response::new(HealthResponse {
            healthy,
            status: if healthy { "ready".to_string() } else { "not ready".to_string() },
        }))
    }

    async fn list_models(&self, _request: Request<ListModelsRequest>) -> Result<Response<ListModelsResponse>, Status> {
        let models = self.state.models.list_all().await;
        let model_infos: Vec<ModelInfo> = models.iter().map(|m| ModelInfo {
            name: m.name.clone(),
            versions: m.versions.clone(),
            ready: m.ready,
        }).collect();

        Ok(Response::new(ListModelsResponse { models: model_infos }))
    }
}
