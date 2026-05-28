//! Ingest Service - RTSP/RTP ingestion, reordering, and media processing
//! 
//! This service handles:
//! - RTSP session management
//! - RTP packet reception and jitter buffering
//! - Packet reordering and loss detection
//! - RTCP handler for quality feedback
//! - fMP4 muxing
//! - S3 multipart upload
//! - WebRTC signaling integration

#![warn(rust_2018_idioms)]
#![warn(missing_debug_implementations)]
#![warn(missing_docs)]
#![allow(clippy::new_without_default)]

pub mod config;
pub mod error;
pub mod metrics;
pub mod rtsp;
pub mod rtp;
pub mod rtcp;
pub mod muxer;
pub mod storage;
pub mod webrtc;
pub mod api;

use std::sync::Arc;
use std::time::Duration;

use tokio::sync::{broadcast, mpsc};
use tracing::{info, error};
use tracing_subscriber::{layer::SubscriberExt, util::SubscriberInitExt};

pub use config::Config;
pub use error::{Error, Result};

/// Application state shared across all subsystems
#[derive(Debug)]
pub struct AppState {
    pub config: Arc<Config>,
    pub rtsp_manager: Arc<rtsp::RtspSessionManager>,
    pub rtp_buffer: Arc<rtp::JitterBuffer>,
    pub rtcp_handler: Arc<rtcp::RtcpHandler>,
    pub muxer: Arc<muxer::Fmp4Muxer>,
    pub storage: Arc<storage::S3Uploader>,
    pub metrics_handle: metrics::MetricsHandle,
    pub shutdown_tx: broadcast::Sender<()>,
}

impl AppState {
    /// Create a new application state with all subsystems initialized
    pub async fn new(config: Config) -> Result<Self> {
        let config = Arc::new(config);
        let (shutdown_tx, _) = broadcast::channel::<()>(16);
        
        // Initialize metrics
        let metrics_handle = metrics::MetricsHandle::new(&config.metrics_bind_addr)?;
        
        // Initialize RTSP session manager
        let rtsp_manager = Arc::new(rtsp::RtspSessionManager::new(
            config.rtsp_port,
            shutdown_tx.subscribe(),
        ));
        
        // Initialize RTP jitter buffer
        let rtp_buffer = Arc::new(rtp::JitterBuffer::new(
            config.rtp_buffer_size,
            config.rtp_max_latency_ms,
        ));
        
        // Initialize RTCP handler
        let rtcp_handler = Arc::new(rtcp::RtcpHandler::new(
            Arc::clone(&rtp_buffer),
        ));
        
        // Initialize fMP4 muxer
        let muxer = Arc::new(muxer::Fmp4Muxer::new(
            config.segment_duration_ms,
            config.init_segment_path.as_deref(),
        ));
        
        // Initialize S3 uploader
        let storage = Arc::new(storage::S3Uploader::new(
            config.s3_bucket.clone(),
            config.s3_region.clone(),
            config.s3_endpoint.clone(),
            config.s3_access_key.clone(),
            config.s3_secret_key.clone(),
        ).await?);
        
        Ok(Self {
            config,
            rtsp_manager,
            rtp_buffer,
            rtcp_handler,
            muxer,
            storage,
            metrics_handle,
            shutdown_tx,
        })
    }
    
    /// Get a clone of the shutdown receiver
    pub fn shutdown_rx(&self) -> broadcast::Receiver<()> {
        self.shutdown_tx.subscribe()
    }
}

/// Main entry point for the ingest service
pub async fn run(config: Config) -> Result<()> {
    // Initialize tracing
    let filter = tracing_subscriber::EnvFilter::try_from_default_env()
        .unwrap_or_else(|_| "ingest_service=info,tower_http=debug".parse().unwrap());
    
    tracing_subscriber::registry()
        .with(filter)
        .with(tracing_subscriber::fmt::layer().json())
        .init();
    
    info!("Starting ingest service");
    
    // Create application state
    let state = Arc::new(AppState::new(config).await?);
    
    // Spawn subsystem tasks
    let mut join_set = tokio::task::JoinSet::new();
    
    // RTSP server task
    let rtsp_manager = Arc::clone(&state.rtsp_manager);
    join_set.spawn(async move {
        if let Err(e) = rtsp_manager.run().await {
            error!("RTSP manager error: {}", e);
        }
    });
    
    // RTP receiver task
    let rtp_buffer = Arc::clone(&state.rtp_buffer);
    let rtsp_manager = Arc::clone(&state.rtsp_manager);
    let mut shutdown_rx = state.shutdown_rx();
    join_set.spawn(async move {
        tokio::select! {
            result = rtp_buffer.receive_loop(Arc::clone(&rtsp_manager)) => {
                if let Err(e) = result {
                    error!("RTP receiver error: {}", e);
                }
            }
            _ = shutdown_rx.recv() => {
                info!("RTP receiver shutting down");
            }
        }
    });
    
    // RTCP handler task
    let rtcp_handler = Arc::clone(&state.rtcp_handler);
    let mut shutdown_rx = state.shutdown_rx();
    join_set.spawn(async move {
        tokio::select! {
            result = rtcp_handler.run() => {
                if let Err(e) = result {
                    error!("RTCP handler error: {}", e);
                }
            }
            _ = shutdown_rx.recv() => {
                info!("RTCP handler shutting down");
            }
        }
    });
    
    // Muxer task
    let muxer = Arc::clone(&state.muxer);
    let rtp_buffer = Arc::clone(&state.rtp_buffer);
    let storage = Arc::clone(&state.storage);
    let mut shutdown_rx = state.shutdown_rx();
    join_set.spawn(async move {
        tokio::select! {
            result = muxer.run(Arc::clone(&rtp_buffer), Arc::clone(&storage)) => {
                if let Err(e) = result {
                    error!("Muxer error: {}", e);
                }
            }
            _ = shutdown_rx.recv() => {
                info!("Muxer shutting down");
            }
        }
    });
    
    // Storage uploader task
    let storage = Arc::clone(&state.storage);
    let muxer = Arc::clone(&state.muxer);
    let mut shutdown_rx = state.shutdown_rx();
    join_set.spawn(async move {
        tokio::select! {
            result = storage.run(Arc::clone(&muxer)) => {
                if let Err(e) = result {
                    error!("Storage uploader error: {}", e);
                }
            }
            _ = shutdown_rx.recv() => {
                info!("Storage uploader shutting down");
            }
        }
    });
    
    // API server task
    let api_state = Arc::clone(&state);
    let mut shutdown_rx = state.shutdown_rx();
    join_set.spawn(async move {
        tokio::select! {
            result = api::run_server(api_state) => {
                if let Err(e) = result {
                    error!("API server error: {}", e);
                }
            }
            _ = shutdown_rx.recv() => {
                info!("API server shutting down");
            }
        }
    });
    
    // Wait for shutdown signal
    tokio::signal::ctrl_c().await?;
    info!("Received shutdown signal");
    
    // Broadcast shutdown to all subsystems
    let _ = state.shutdown_tx.send(());
    
    // Wait for all tasks to complete
    while let Some(result) = join_set.join_next().await {
        match result {
            Ok(_) => info!("Task completed"),
            Err(e) => error!("Task failed: {}", e),
        }
    }
    
    info!("Ingest service shutdown complete");
    Ok(())
}
