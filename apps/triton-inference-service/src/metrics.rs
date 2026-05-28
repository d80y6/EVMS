use metrics::{counter, histogram, gauge};

/// Prometheus metrics for inference service
pub fn register_metrics() {
    // Inference latency histogram
    histogram!("inference_latency_seconds", "type" => "total");
    histogram!("inference_latency_seconds", "type" => "queue");
    histogram!("inference_latency_seconds", "type" => "execution");
    
    // Batch size histogram
    histogram!("batch_size");
    
    // Queue depth gauge
    gauge!("queue_depth");
    
    // Request counters
    counter!("requests_total", "status" => "success");
    counter!("requests_total", "status" => "error");
    counter!("requests_total", "status" => "timeout");
    
    // Model-specific metrics
    counter!("model_inferences_total", "model" => "default");
    histogram!("model_inference_latency_seconds", "model" => "default");
    
    // GPU metrics
    gauge!("gpu_memory_used_bytes");
    gauge!("gpu_utilization_percent");
    gauge!("gpu_temperature_celsius");
    
    // Error counters
    counter!("errors_total", "type" => "preprocessing");
    counter!("errors_total", "type" => "postprocessing");
    counter!("errors_total", "type" => "model");
    counter!("errors_total", "type" => "connection");
}

pub fn record_inference_latency(latency_secs: f64, latency_type: &str) {
    histogram!("inference_latency_seconds", "type" => latency_type).record(latency_secs);
}

pub fn record_batch_size(size: usize) {
    histogram!("batch_size").record(size as f64);
}

pub fn set_queue_depth(depth: usize) {
    gauge!("queue_depth").set(depth as f64);
}

pub fn increment_request_counter(status: &str) {
    counter!("requests_total", "status" => status).increment(1);
}

pub fn increment_model_inference(model_name: &str) {
    counter!("model_inferences_total", "model" => model_name).increment(1);
}

pub fn record_model_latency(model_name: &str, latency_secs: f64) {
    histogram!("model_inference_latency_seconds", "model" => model_name).record(latency_secs);
}

pub fn set_gpu_memory_used(bytes: u64) {
    gauge!("gpu_memory_used_bytes").set(bytes as f64);
}

pub fn set_gpu_utilization(percent: f32) {
    gauge!("gpu_utilization_percent").set(percent as f64);
}

pub fn set_gpu_temperature(celsius: f32) {
    gauge!("gpu_temperature_celsius").set(celsius as f64);
}

pub fn increment_error_counter(error_type: &str) {
    counter!("errors_total", "type" => error_type).increment(1);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_metrics_recording() {
        register_metrics();
        
        record_inference_latency(0.05, "total");
        record_batch_size(16);
        set_queue_depth(5);
        increment_request_counter("success");
        increment_model_inference("resnet50");
        record_model_latency("resnet50", 0.03);
        set_gpu_memory_used(1024 * 1024 * 1024);
        set_gpu_utilization(75.5);
        set_gpu_temperature(65.0);
        increment_error_counter("preprocessing");
        
        // If we get here without panicking, metrics are working
        assert!(true);
    }
}
