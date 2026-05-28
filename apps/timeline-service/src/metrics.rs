use metrics::{counter, gauge, histogram};
use metrics_exporter_prometheus::{PrometheusBuilder, PrometheusHandle};
use crate::config::Config;

pub fn init_metrics(config: &Config) -> Result<(), Box<dyn std::error::Error>> {
    let recorder = PrometheusBuilder::new()
        .set_quantiles(&[0.0, 0.5, 0.9, 0.95, 0.99, 1.0])?
        .build_recorder();
    metrics::set_boxed_recorder(Box::new(recorder))?;
    Ok(())
}

pub fn get_metrics_handle() -> PrometheusHandle {
    PrometheusBuilder::new().build_recorder().handle()
}

pub fn record_clock_skew(skew_ms: f64) {
    gauge!("timeline_clock_skew_ms").set(skew_ms);
}

pub fn record_alignment_offset(stream_id: &str, offset_ms: f64) {
    gauge!("timeline_alignment_offset_ms", "stream_id" => stream_id.to_string()).set(offset_ms);
}

pub fn record_segment_added(stream_id: &str, duration_ms: i64) {
    counter!("timeline_segments_total", "stream_id" => stream_id.to_string()).increment(1);
    histogram!("timeline_segment_duration_ms", "stream_id" => stream_id.to_string()).record(duration_ms as f64);
}

pub fn record_gap_detected(stream_id: &str, gap_ms: i64) {
    counter!("timeline_gaps_total", "stream_id" => stream_id.to_string()).increment(1);
    histogram!("timeline_gap_duration_ms", "stream_id" => stream_id.to_string()).record(gap_ms as f64);
}
