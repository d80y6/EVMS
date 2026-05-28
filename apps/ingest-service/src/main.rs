//! Main entry point for the ingest service

use ingest_service::{Config, run};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Load configuration
    let config = Config::load().unwrap_or_default();
    
    // Initialize tracing
    let filter = tracing_subscriber::EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| "ingest_service=info,tower_http=debug".parse().unwrap());
    
    tracing_subscriber::registry()
        .with(filter)
        .with(tracing_subscriber::fmt::layer().json())
        .init();
    
    // Run the service
    if let Err(e) = run(config).await {
        eprintln!("Service error: {}", e);
        std::process::exit(1);
    }
    
    Ok(())
}
