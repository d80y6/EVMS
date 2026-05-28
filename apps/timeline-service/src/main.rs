use tokio::sync::mpsc;
use tracing::{info, error};
use crate::config::Config;
use crate::api::start_api_server;
use crate::metrics::init_metrics;

mod config;
mod error;
mod clock;
mod aligner;
mod segment_manager;
mod api;
mod metrics;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    tracing_subscriber::fmt()
        .with_env_filter(
            tracing_subscriber::EnvFilter::from_default_env()
                .add_directive("timeline_service=info".parse()?),
        )
        .init();

    let config = Config::from_env()?;
    info!("Starting Timeline Service");

    init_metrics(&config)?;
    start_api_server(config).await?;
    
    Ok(())
}
