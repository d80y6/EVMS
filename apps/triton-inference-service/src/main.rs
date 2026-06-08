//! Triton Inference Service - Main Entry Point
//!
//! Async GPU inference engine with dynamic batching

use std::sync::Arc;
use std::time::Duration;
use tokio::sync::Notify;
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[path = "lib.rs"]
mod lib;
pub use lib::*;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "info".into()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    // Load configuration
    let config = config::Config::load()?;
    
    tracing::info!("Starting Triton Inference Service");
    tracing::info!("Triton URL: {}", config.triton_url);
    tracing::info!("HTTP Port: {}", config.http_port);
    tracing::info!("gRPC Port: {}", config.grpc_port);
    tracing::info!("Max Batch Size: {}", config.max_batch_size);
    tracing::info!("Batch Timeout: {}ms", config.batch_timeout_ms);

    // Create application state
    let state = create_state(&config)?;
    let state = Arc::new(state);

    // Register metrics
    metrics::register_metrics();

    // Spawn metrics exporter
    let metrics_handle = metrics_exporter_prometheus::PrometheusBuilder::new()
        .install_recorder()?;

    // Shutdown notification for graceful shutdown coordination
    let shutdown = Arc::new(Notify::new());

    // Signal handler for graceful shutdown
    let sig_shutdown = shutdown.clone();
    tokio::spawn(async move {
        let mut term = tokio::signal::unix::signal(tokio::signal::unix::SignalKind::terminate())
            .expect("failed to register SIGTERM handler");
        tokio::select! {
            _ = tokio::signal::ctrl_c() => {}
            _ = term.recv() => {}
        }
        tracing::info!("Shutdown signal received, starting graceful shutdown");
        sig_shutdown.notify_one();
    });

    // Create HTTP server
    let http_addr = std::net::SocketAddr::from(([0, 0, 0, 0], config.http_port));
    let http_app = api::create_router(state.clone());
    tracing::info!("Starting HTTP server on {}", http_addr);

    let http_shutdown = shutdown.clone();
    let listener = tokio::net::TcpListener::bind(&http_addr).await?;
    let http_server = axum::serve(listener, http_app.into_make_service())
        .with_graceful_shutdown(async move {
            http_shutdown.notified().await;
        });

    // Create gRPC server
    let grpc_addr = std::net::SocketAddr::from(([0, 0, 0, 0], config.grpc_port));
    let grpc_service = crate::grpc::TritonGrpcService::new(state.clone());

    let grpc_shutdown = shutdown.clone();
    let grpc_server = tonic::transport::Server::builder()
        .add_service(crate::grpc::InferenceServiceServer::new(grpc_service))
        .serve_with_shutdown(grpc_addr, async move {
            grpc_shutdown.notified().await;
        });

    tracing::info!("Starting gRPC server on {}", grpc_addr);

    // Start background tasks
    let state_clone = state.clone();
    tokio::spawn(async move {
        loop {
            tokio::time::sleep(std::time::Duration::from_secs(10)).await;
            
            let queue_size = state_clone.batcher.queue_size().await;
            let active = state_clone.scheduler.active_count();
            let available = state_clone.scheduler.available_slots();
            
            tracing::debug!(
                "Queue: {}, Active: {}, Available: {}",
                queue_size, active, available
            );
            
            crate::metrics::set_queue_depth(queue_size);
        }
    });

    // Run HTTP and gRPC servers concurrently
    tokio::select! {
        result = http_server => {
            if let Err(e) = result {
                tracing::error!("HTTP server error: {}", e);
            }
        }
        result = grpc_server => {
            if let Err(e) = result {
                tracing::error!("gRPC server error: {}", e);
            }
        }
    }

    // Wait for active requests to complete (max 30s)
    tokio::time::sleep(Duration::from_secs(30)).await;
    tracing::info!("Triton Inference Service shutdown complete");

    Ok(())
}
