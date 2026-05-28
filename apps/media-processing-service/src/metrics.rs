use metrics::{counter, gauge, histogram};
use metrics_exporter_prometheus::{PrometheusBuilder, PrometheusHandle};
use crate::config::Config;

pub fn init_metrics(config: &Config) -> Result<(), Box<dyn std::error::Error>> {
    let recorder = PrometheusBuilder::new()
        .set_quantiles(&[0.0, 0.5, 0.9, 0.95, 0.99, 1.0])?
        .build_recorder();
    let handle = recorder.handle();
    metrics::set_boxed_recorder(Box::new(recorder))?;
    Ok(())
}

pub fn get_metrics_handle() -> PrometheusHandle {
    let handle = PrometheusBuilder::new().build_recorder().handle();
    handle
}

pub fn record_frame_processed(pipeline_id: &str, duration_ms: f64) {
    counter!("media_frames_processed_total", "pipeline_id" => pipeline_id.to_string()).increment(1);
    histogram!("media_frame_processing_duration_ms", "pipeline_id" => pipeline_id.to_string()).record(duration_ms);
}

pub fn record_inference_request(pipeline_id: &str, model_name: &str, duration_ms: f64) {
    counter!("media_inference_requests_total", "pipeline_id" => pipeline_id.to_string(), "model" => model_name.to_string()).increment(1);
    histogram!("media_inference_duration_ms", "pipeline_id" => pipeline_id.to_string(), "model" => model_name.to_string()).record(duration_ms);
}

pub fn set_pipeline_state(pipeline_id: &str, state: &str) {
    gauge!("media_pipeline_state", "pipeline_id" => pipeline_id.to_string(), "state" => state.to_string()).set(1.0);
}

pub fn record_pipeline_error(pipeline_id: &str, error_type: &str) {
    counter!("media_pipeline_errors_total", "pipeline_id" => pipeline_id.to_string(), "error_type" => error_type.to_string()).increment(1);
}

pub fn record_buffer_depth(pipeline_id: &str, depth: u64) {
    gauge!("media_buffer_depth", "pipeline_id" => pipeline_id.to_string()).set(depth as f64);
}
