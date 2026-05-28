//! Edge Sync Service - Main Entry Point

use edge_sync::{init, Config};
use std::sync::Arc;
use tracing::{info, error};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    // Initialize tracing
    tracing_subscriber::registry()
        .with(tracing_subscriber::EnvFilter::new(
            std::env::var("RUST_LOG").unwrap_or_else(|_| "edge_sync=info,tower_http=debug".into()),
        ))
        .with(tracing_subscriber::fmt::layer())
        .init();

    info!("Starting edge sync service");

    // Load configuration
    let config = Config::from_env().map_err(|e| {
        error!("Failed to load configuration: {}", e);
        e
    })?;

    config.validate().map_err(|e| {
        error!("Configuration validation failed: {}", e);
        e
    })?;

    info!("Configuration loaded for device: {}", config.device_id);

    // Initialize application state
    let state = init(config).await.map_err(|e| {
        error!("Failed to initialize service: {}", e);
        e
    })?;

    let state = Arc::new(state);

    // Create API state
    let api_state = edge_sync::api::ApiState::new(state.clone());

    // Start HTTP server
    let http_addr: std::net::SocketAddr = format!(
        "{}:{}",
        state.config.network.bind_address, state.config.network.http_port
    )
    .parse()?;

    let http_state = api_state.clone();
    let http_server = tokio::spawn(async move {
        let app = edge_sync::api::create_router(http_state);
        
        info!("Starting HTTP server on {}", http_addr);
        
        let listener = tokio::net::TcpListener::bind(http_addr).await?;
        axum::serve(listener, app).await?;
        
        Ok::<(), Box<dyn std::error::Error + Send + Sync>>(())
    });

    // Start gRPC server
    let grpc_addr: std::net::SocketAddr = format!(
        "{}:{}",
        state.config.network.bind_address, state.config.network.grpc_port
    )
    .parse()?;

    let grpc_state = api_state.clone();
    let grpc_server = tokio::spawn(async move {
        use edge_sync::grpc::sync::edge_sync_service_server::EdgeSyncServiceServer;
        use edge_sync::grpc::EdgeSyncGrpcService;
        
        let service = EdgeSyncGrpcService::new(grpc_state);
        let server = tonic::transport::Server::builder()
            .add_service(EdgeSyncServiceServer::new(service));

        info!("Starting gRPC server on {}", grpc_addr);
        
        server.serve(grpc_addr).await?;
        
        Ok::<(), Box<dyn std::error::Error + Send + Sync>>(())
    });

    // Start periodic sync task
    let sync_state = state.clone();
    let sync_interval = state.config.sync.interval_secs;
    let sync_task = tokio::spawn(async move {
        let mut interval = tokio::time::interval(tokio::time::Duration::from_secs(sync_interval));
        
        loop {
            interval.tick().await;
            
            if sync_state.offline_queue.stats().await.pending > 0 {
                info!("Running periodic sync");
                
                match sync_state.sync_engine.process_queue().await {
                    Ok(result) => {
                        info!(
                            "Sync completed: {} success, {} failures, {} conflicts",
                            result.success_count, result.failure_count, result.conflicts.len()
                        );
                    }
                    Err(e) => {
                        error!("Sync failed: {}", e);
                    }
                }
            }
        }
    });

    info!("Edge sync service started successfully");

    // Wait for servers
    tokio::select! {
        result = http_server => {
            match result {
                Ok(Ok(())) => info!("HTTP server stopped"),
                Ok(Err(e)) => error!("HTTP server error: {}", e),
                Err(e) => error!("HTTP server task failed: {}", e),
            }
        }
        result = grpc_server => {
            match result {
                Ok(Ok(())) => info!("gRPC server stopped"),
                Ok(Err(e)) => error!("gRPC server error: {}", e),
                Err(e) => error!("gRPC server task failed: {}", e),
            }
        }
        _ = sync_task => {
            info!("Sync task stopped");
        }
    }

    Ok(())
}
