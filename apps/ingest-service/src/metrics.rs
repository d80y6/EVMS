//! Metrics collection and Prometheus export

use metrics::{counter, gauge, histogram};
use metrics_exporter_prometheus::{PrometheusBuilder, PrometheusHandle};
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::net::TcpListener;
use tokio::sync::Mutex;
use tracing::info;

/// Metrics handle for the service
#[derive(Debug)]
pub struct MetricsHandle {
    pub recorder_handle: PrometheusHandle,
    _shutdown: Arc<Mutex<bool>>,
}

impl MetricsHandle {
    /// Create a new metrics handle and start the Prometheus exporter
    pub fn new(bind_addr: &str) -> crate::Result<Self> {
        let addr: SocketAddr = bind_addr
            .parse()
            .map_err(|e| crate::Error::Metrics(format!("Invalid bind address: {}", e)))?;
        
        let recorder = PrometheusBuilder::new()
            .build_recorder();
        let handle = recorder.handle();
        
        metrics::set_boxed_recorder(Box::new(recorder))
            .map_err(|e| crate::Error::Metrics(format!("Failed to set recorder: {}", e)))?;
        
        let shutdown = Arc::new(Mutex::new(false));
        let shutdown_clone = Arc::clone(&shutdown);
        
        // Spawn metrics HTTP server
        tokio::spawn(async move {
            let listener = TcpListener::bind(addr).await.ok()?;
            
            info!("Metrics server listening on {}", addr);
            
            loop {
                let (mut stream, _) = listener.accept().await.ok()?;
                
                let shutdown_flag = Arc::clone(&shutdown_clone);
                tokio::spawn(async move {
                    use tokio::io::AsyncWriteExt;
                    
                    let mut buf = [0u8; 1024];
                    let _ = stream.readable().await;
                    let _ = stream.try_read(&mut buf).ok();
                    
                    // Check if shutdown requested
                    if *shutdown_flag.lock().await {
                        return;
                    }
                    
                    let response = "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\n";
                    let _ = stream.write_all(response.as_bytes()).await;
                    let _ = stream.write_all(handle.render().as_bytes()).await;
                    let _ = stream.shutdown().await;
                });
            }
        });
        
        Ok(Self {
            recorder_handle: handle,
            _shutdown: shutdown,
        })
    }
}

/// Record an RTP packet received
#[inline]
pub fn record_rtp_packet_received(ssrc: u32, payload_type: u8, bytes: usize) {
    counter!("rtp_packets_received_total", "ssrc" => ssrc.to_string(), "payload_type" => payload_type.to_string()).increment(1);
    counter!("rtp_bytes_received_total", "ssrc" => ssrc.to_string()).increment(bytes as u64);
    histogram!("rtp_packet_size_bytes", "ssrc" => ssrc.to_string()).record(bytes as f64);
}

/// Record an RTP packet dropped
#[inline]
pub fn record_rtp_packet_dropped(ssrc: u32, reason: &'static str) {
    counter!("rtp_packets_dropped_total", "ssrc" => ssrc.to_string(), "reason" => reason).increment(1);
}

/// Record an RTCP packet received
#[inline]
pub fn record_rtcp_packet_received(pt: u8, bytes: usize) {
    counter!("rtcp_packets_received_total", "packet_type" => pt.to_string()).increment(1);
    counter!("rtcp_bytes_received_total").increment(bytes as u64);
}

/// Record buffer occupancy
#[inline]
pub fn record_buffer_occupancy(ssrc: u32, count: usize, latency_ms: f64) {
    gauge!("rtp_buffer_occupancy", "ssrc" => ssrc.to_string()).set(count as f64);
    histogram!("rtp_buffer_latency_ms", "ssrc" => ssrc.to_string()).record(latency_ms);
}

/// Record a reordered packet
#[inline]
pub fn record_packet_reordered(ssrc: u32, gap: u16) {
    counter!("rtp_packets_reordered_total", "ssrc" => ssrc.to_string()).increment(1);
    histogram!("rtp_reorder_gap", "ssrc" => ssrc.to_string()).record(gap as f64);
}

/// Record packet loss detected
#[inline]
pub fn record_packet_loss(ssrc: u32, lost_count: u16) {
    counter!("rtp_packets_lost_total", "ssrc" => ssrc.to_string()).increment(lost_count as u64);
}

/// Record a muxed segment
#[inline]
pub fn record_segment_muxed(duration_ms: u64, bytes: usize) {
    counter!("segments_muxed_total").increment(1);
    histogram!("segment_duration_ms").record(duration_ms as f64);
    histogram!("segment_size_bytes").record(bytes as f64);
}

/// Record an S3 upload
#[inline]
pub fn record_s3_upload(bytes: usize, duration_ms: u64, success: bool) {
    counter!("s3_uploads_total", "success" => success.to_string()).increment(1);
    histogram!("s3_upload_size_bytes").record(bytes as f64);
    histogram!("s3_upload_duration_ms").record(duration_ms as f64);
}

/// Record RTSP session events
#[inline]
pub fn record_rtsp_session_created() {
    counter!("rtsp_sessions_created_total").increment(1);
    gauge!("rtsp_sessions_active").increment(1.0);
}

#[inline]
pub fn record_rtsp_session_destroyed() {
    gauge!("rtsp_sessions_active").decrement(1.0);
}

/// Record WebRTC signaling events
#[inline]
pub fn record_webrtc_offer_created() {
    counter!("webrtc_offers_created_total").increment(1);
}

#[inline]
pub fn record_webrtc_answer_received() {
    counter!("webrtc_answers_received_total").increment(1);
}

/// Record inference request
#[inline]
pub fn record_inference_request(model: &str, duration_ms: f64, success: bool) {
    counter!("inference_requests_total", "model" => model.to_string(), "success" => success.to_string()).increment(1);
    histogram!("inference_duration_ms", "model" => model.to_string()).record(duration_ms);
}
