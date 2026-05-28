use tokio::sync::mpsc;
use tracing::{info, error};
use crate::config::Config;
use crate::pipeline::PipelineManager;
use crate::api::start_api_server;
use crate::metrics::init_metrics;

mod config;
mod error;
mod pipeline;
mod preprocess;
mod inference;
mod postprocess;
mod api;
mod metrics;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("media_processing_service=info".parse()?),
        )
        .init();

    let config = Config::from_env()?;
    info!("Starting Media Processing Service");

    init_metrics(&config)?;

    let (pipeline_cmd_tx, pipeline_cmd_rx) = mpsc::channel(100);
    let manager = PipelineManager::new(config.clone(), pipeline_cmd_tx);

    let api_handle = tokio::spawn(async move {
        if let Err(e) = start_api_server(config, pipeline_cmd_rx).await {
            error!("API server failed: {}", e);
        }
    });

    api_handle.await?;
    
    Ok(())
}
