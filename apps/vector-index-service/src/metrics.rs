use metrics::{counter, histogram, gauge};

pub fn register_metrics() {
    counter!("vector_inserts_total");
    counter!("vector_searches_total");
    histogram!("search_latency_seconds");
    histogram!("index_build_time_seconds");
    gauge!("cache_size");
    gauge!("cache_hit_rate");
    counter!("cache_hits_total");
    counter!("cache_misses_total");
    gauge!("milvus_connections");
    counter!("errors_total", "type" => "index");
    counter!("errors_total", "type" => "search");
    counter!("errors_total", "type" => "connection");
}

pub fn record_search_latency(latency_secs: f64) {
    histogram!("search_latency_seconds").record(latency_secs);
}

pub fn increment_searches() {
    counter!("vector_searches_total").increment(1);
}

pub fn increment_inserts() {
    counter!("vector_inserts_total").increment(1);
}

pub fn set_cache_size(size: usize) {
    gauge!("cache_size").set(size as f64);
}

pub fn record_cache_hit() {
    counter!("cache_hits_total").increment(1);
}

pub fn record_cache_miss() {
    counter!("cache_misses_total").increment(1);
}
