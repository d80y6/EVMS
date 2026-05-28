use tokio::sync::mpsc;
use tracing::{info, error, warn};
use uuid::Uuid;
use std::collections::HashMap;
use std::sync::Arc;
use parking_lot::RwLock;

use crate::config::Config;
use crate::error::{Result, MediaProcessingError};
use crate::preprocess::{FramePreprocessor, PreprocessConfig};
use crate::inference::{InferenceClient, InferenceRequest, TensorInput, DataType};
use crate::postprocess::{Detection, non_max_suppression, apply_confidence_threshold};

#[derive(Debug, Clone)]
pub struct PipelineConfig {
    pub id: String,
    pub source_uri: String,
    pub model_name: String,
    pub preprocess: PreprocessConfig,
    pub confidence_threshold: f32,
    pub nms_threshold: f32,
}

#[derive(Debug, Clone)]
pub enum PipelineState {
    Created,
    Running,
    Stopped,
    Error(String),
}

pub struct Pipeline {
    pub config: PipelineConfig,
    pub state: PipelineState,
    pub frame_count: u64,
    pub error_count: u64,
}

pub struct PipelineManager {
    config: Config,
    pipelines: Arc<RwLock<HashMap<String, Pipeline>>>,
    cmd_tx: mpsc::Sender<PipelineCommand>,
}

#[derive(Debug)]
pub enum PipelineCommand {
    Create(PipelineConfig, mpsc::Sender<Result<String>>),
    Start(String, mpsc::Sender<Result<()>>),
    Stop(String, mpsc::Sender<Result<()>>),
    Delete(String, mpsc::Sender<Result<()>>),
    GetStatus(String, mpsc::Sender<Option<Pipeline>>),
    List(mpsc::Sender<Vec<Pipeline>>),
}

impl PipelineManager {
    pub fn new(config: Config, cmd_tx: mpsc::Sender<PipelineCommand>) -> Self {
        let manager = PipelineManager {
            config,
            pipelines: Arc::new(RwLock::new(HashMap::new())),
            cmd_tx,
        };

        let cmd_rx = tokio::sync::mpsc::channel(100);
        tokio::spawn(run_command_loop(manager.pipelines.clone(), cmd_rx.1));

        manager
    }

    pub async fn create_pipeline(&self, config: PipelineConfig) -> Result<String> {
        let (tx, mut rx) = mpsc::channel(1);
        self.cmd_tx.send(PipelineCommand::Create(config, tx)).await
            .map_err(|e| MediaProcessingError::PipelineState(e.to_string()))?;
        
        rx.recv().await
            .ok_or_else(|| MediaProcessingError::PipelineState("Channel closed".to_string()))?
            .map(|id| { info!("Created pipeline {}", id); id })
    }

    pub async fn start_pipeline(&self, id: &str) -> Result<()> {
        let (tx, mut rx) = mpsc::channel(1);
        self.cmd_tx.send(PipelineCommand::Start(id.to_string(), tx)).await
            .map_err(|e| MediaProcessingError::PipelineState(e.to_string()))?;
        
        rx.recv().await
            .ok_or_else(|| MediaProcessingError::PipelineState("Channel closed".to_string()))??;
        Ok(())
    }

    pub async fn stop_pipeline(&self, id: &str) -> Result<()> {
        let (tx, mut rx) = mpsc::channel(1);
        self.cmd_tx.send(PipelineCommand::Stop(id.to_string(), tx)).await
            .map_err(|e| MediaProcessingError::PipelineState(e.to_string()))?;
        
        rx.recv().await
            .ok_or_else(|| MediaProcessingError::PipelineState("Channel closed".to_string()))??;
        Ok(())
    }

    pub async fn delete_pipeline(&self, id: &str) -> Result<()> {
        let (tx, mut rx) = mpsc::channel(1);
        self.cmd_tx.send(PipelineCommand::Delete(id.to_string(), tx)).await
            .map_err(|e| MediaProcessingError::PipelineState(e.to_string()))?;
        
        rx.recv().await
            .ok_or_else(|| MediaProcessingError::PipelineState("Channel closed".to_string()))??;
        Ok(())
    }

    pub async fn list_pipelines(&self) -> Vec<Pipeline> {
        let (tx, mut rx) = mpsc::channel(1);
        let _ = self.cmd_tx.send(PipelineCommand::List(tx)).await;
        rx.recv().await.unwrap_or_default()
    }
}

async fn run_command_loop(
    pipelines: Arc<RwLock<HashMap<String, Pipeline>>>,
    mut cmd_rx: mpsc::Receiver<PipelineCommand>,
) {
    while let Some(cmd) = cmd_rx.recv().await {
        match cmd {
            PipelineCommand::Create(config, tx) => {
                let id = config.id.clone();
                if pipelines.read().contains_key(&id) {
                    let _ = tx.send(Err(MediaProcessingError::PipelineExists(id))).await;
                } else {
                    let pipeline = Pipeline {
                        config,
                        state: PipelineState::Created,
                        frame_count: 0,
                        error_count: 0,
                    };
                    pipelines.write().insert(id.clone(), pipeline);
                    let _ = tx.send(Ok(id)).await;
                }
            }
            PipelineCommand::Start(id, tx) => {
                let mut pipelines_write = pipelines.write();
                if let Some(pipeline) = pipelines_write.get_mut(&id) {
                    pipeline.state = PipelineState::Running;
                    let _ = tx.send(Ok(())).await;
                } else {
                    let _ = tx.send(Err(MediaProcessingError::PipelineNotFound(id))).await;
                }
            }
            PipelineCommand::Stop(id, tx) => {
                let mut pipelines_write = pipelines.write();
                if let Some(pipeline) = pipelines_write.get_mut(&id) {
                    pipeline.state = PipelineState::Stopped;
                    let _ = tx.send(Ok(())).await;
                } else {
                    let _ = tx.send(Err(MediaProcessingError::PipelineNotFound(id))).await;
                }
            }
            PipelineCommand::Delete(id, tx) => {
                let mut pipelines_write = pipelines.write();
                if pipelines_write.remove(&id).is_some() {
                    let _ = tx.send(Ok(())).await;
                } else {
                    let _ = tx.send(Err(MediaProcessingError::PipelineNotFound(id))).await;
                }
            }
            PipelineCommand::GetStatus(id, tx) => {
                let pipelines_read = pipelines.read();
                let status = pipelines_read.get(&id).cloned();
                let _ = tx.send(status).await;
            }
            PipelineCommand::List(tx) => {
                let pipelines_read = pipelines.read();
                let list: Vec<Pipeline> = pipelines_read.values().cloned().collect();
                let _ = tx.send(list).await;
            }
        }
    }
}
